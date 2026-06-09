package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fzerorubigd/life-tracker/handler"
)

type stubHandler struct {
	name string
	run  func(payload []byte) error
}

func (s stubHandler) Name() string                          { return s.name }
func (s stubHandler) Run(_ context.Context, p []byte) error { return s.run(p) }

// TestEmailActions pins test-mode safety (#12): only email actions are
// kept, so a test run never fires destructive handlers.
func TestEmailActions(t *testing.T) {
	in := []Action{
		{Handler: "email", PayloadFile: "r1.gpg"},
		{Handler: "http-delete", PayloadFile: "d.gpg"},
		{Handler: "email", PayloadFile: "r2.gpg"},
		{Handler: "gh-post", PayloadFile: "p.gpg"},
	}
	out := emailActions(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 email actions, got %d (%+v)", len(out), out)
	}
	for _, a := range out {
		if a.Handler != "email" {
			t.Errorf("non-email action leaked through: %s", a.Handler)
		}
	}
}

// TestDispatch_Isolation pins the core trigger contract: every action is
// attempted, and a failing/panicking/unresolved action never aborts a
// later one.
func TestDispatch_Isolation(t *testing.T) {
	var ran []string
	mk := func(name string, fn func([]byte) error) handler.Handler {
		return stubHandler{name: name, run: func(p []byte) error {
			ran = append(ran, name)
			return fn(p)
		}}
	}
	reg := map[string]handler.Handler{
		"ok":    mk("ok", func([]byte) error { return nil }),
		"fail":  mk("fail", func([]byte) error { return errors.New("boom") }),
		"panic": mk("panic", func([]byte) error { panic("kaboom") }),
		"late":  mk("late", func([]byte) error { return nil }),
	}
	resolve := func(n string) (handler.Handler, bool) { h, ok := reg[n]; return h, ok }

	actions := []Action{
		{Handler: "ok", PayloadFile: "a.gpg"},
		{Handler: "fail", PayloadFile: "b.gpg"},
		{Handler: "panic", PayloadFile: "c.gpg"},
		{Handler: "ghost", PayloadFile: "d.gpg"},    // unregistered
		{Handler: "ok", PayloadFile: "missing.gpg"}, // no decrypted payload
		{Handler: "", PayloadFile: "e.gpg"},         // empty handler name
		{Handler: "late", PayloadFile: "f.gpg"},     // must still run
	}
	payloads := map[string][]byte{
		"a.gpg": []byte("A"), "b.gpg": []byte("B"), "c.gpg": []byte("C"), "f.gpg": []byte("F"),
	}

	results := Dispatch(context.Background(), actions, payloads, resolve)

	if len(results) != len(actions) {
		t.Fatalf("got %d results, want %d", len(results), len(actions))
	}
	want := []struct {
		ok     bool
		errSub string
	}{
		{true, ""},
		{false, "boom"},
		{false, "panicked"},
		{false, "no handler registered"},
		{false, "no decrypted payload"},
		{false, "no handler name"},
		{true, ""},
	}
	for i, w := range want {
		if results[i].OK != w.ok {
			t.Errorf("result[%d] (%s) OK=%v want %v (err=%q)", i, results[i].Handler, results[i].OK, w.ok, results[i].Error)
		}
		if w.errSub != "" && !strings.Contains(results[i].Error, w.errSub) {
			t.Errorf("result[%d] error %q does not contain %q", i, results[i].Error, w.errSub)
		}
		if w.ok && results[i].Error != "" {
			t.Errorf("result[%d] unexpected error %q", i, results[i].Error)
		}
	}

	// Isolation: the final 'late' handler ran despite the earlier
	// failure + panic.
	var lateRan bool
	for _, n := range ran {
		if n == "late" {
			lateRan = true
		}
	}
	if !lateRan {
		t.Fatal("late handler did not run — a prior failure aborted dispatch")
	}
}
