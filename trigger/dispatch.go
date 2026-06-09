package main

import (
	"context"
	"fmt"

	"github.com/fzerorubigd/dead-man-switch/handler"
)

// Action is one entry in the death-action list: which handler to run and
// which (encrypted) payload file feeds it.
type Action struct {
	Handler     string `json:"handler"`
	PayloadFile string `json:"payload_file"`
}

// Result is the per-action outcome reported in the trigger summary.
type Result struct {
	Handler     string `json:"handler"`
	PayloadFile string `json:"payload_file"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
}

// emailActions returns only the email actions, used by test-mode (#12)
// so an on-demand test never fires destructive handlers.
func emailActions(actions []Action) []Action {
	out := make([]Action, 0, len(actions))
	for _, a := range actions {
		if a.Handler == "email" {
			out = append(out, a)
		}
	}
	return out
}

// Resolver maps a handler name to its registered Handler. Production
// passes handler.Get; tests inject a map so dispatch is exercised
// without the global registry.
type Resolver func(name string) (handler.Handler, bool)

// Dispatch runs each action's handler against its decrypted payload,
// isolating failures (including panics) so one bad action never aborts
// the others. payloads maps an action's PayloadFile to its decrypted
// bytes. The returned results are 1:1 with actions, in order.
func Dispatch(ctx context.Context, actions []Action, payloads map[string][]byte, resolve Resolver) []Result {
	results := make([]Result, 0, len(actions))
	for _, a := range actions {
		r := Result{Handler: a.Handler, PayloadFile: a.PayloadFile}
		switch {
		case a.Handler == "":
			r.Error = "action has no handler name"
		default:
			if h, ok := resolve(a.Handler); !ok {
				r.Error = "no handler registered for " + a.Handler
			} else if payload, ok := payloads[a.PayloadFile]; !ok {
				r.Error = "no decrypted payload for " + a.PayloadFile
			} else if err := runIsolated(ctx, h, payload); err != nil {
				r.Error = err.Error()
			} else {
				r.OK = true
			}
		}
		results = append(results, r)
	}
	return results
}

// runIsolated runs h.Run, converting a panic into an error so a
// misbehaving handler cannot crash the trigger or block its siblings.
func runIsolated(ctx context.Context, h handler.Handler, payload []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler %q panicked: %v", h.Name(), r)
		}
	}()
	return h.Run(ctx, payload)
}
