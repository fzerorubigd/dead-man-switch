package main

import (
	"strings"
	"testing"

	"github.com/fzerorubigd/life-tracker/internal/dmstate"
)

func TestStatusView(t *testing.T) {
	cases := []struct {
		s            dmstate.Status
		label, class string
	}{
		{dmstate.StatusAlive, "Alive", "alive"},
		{dmstate.StatusWaiting, "Waiting", "waiting"},
		{dmstate.StatusTriggerReady, "Triggered", "triggered"},
		{dmstate.Status("weird"), "Unknown", "unknown"},
		{"", "Unknown", "unknown"},
	}
	for _, c := range cases {
		l, cl, desc := statusView(c.s)
		if l != c.label || cl != c.class {
			t.Errorf("statusView(%q) = %q/%q, want %q/%q", c.s, l, cl, c.label, c.class)
		}
		if desc == "" {
			t.Errorf("statusView(%q) has empty description", c.s)
		}
	}
}

// TestRenderPage_PerStatus pins that the page renders for each lifecycle
// status with the right label/colour-class and always carries the
// gh-pages fallback URL.
func TestRenderPage_PerStatus(t *testing.T) {
	const fallback = "https://fzerorubigd.github.io/life-tracker/"
	for _, s := range []dmstate.Status{dmstate.StatusAlive, dmstate.StatusWaiting, dmstate.StatusTriggerReady, ""} {
		label, class, desc := statusView(s)
		html, err := RenderPage(PageData{
			Label:          label,
			Class:          class,
			Description:    desc,
			LastSignOfLife: "Mon, 01 Jun 2026 00:00:00 UTC",
			FallbackURL:    fallback,
			GeneratedAt:    "now",
		})
		if err != nil {
			t.Fatalf("render %q: %v", s, err)
		}
		for _, want := range []string{label, "status " + class, fallback, "<!doctype html>"} {
			if !strings.Contains(html, want) {
				t.Errorf("status %q page missing %q", s, want)
			}
		}
	}
}

func TestRenderPage_IncludesNoteAndDomain(t *testing.T) {
	html, err := RenderPage(PageData{
		Label: "Alive", Class: "alive", Description: "ok",
		Note: "back in a month", Domain: "am-i-alive.example.test", FallbackURL: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "back in a month") {
		t.Error("note not rendered")
	}
	if !strings.Contains(html, "am-i-alive.example.test") {
		t.Error("domain not rendered")
	}
}

// TestRenderPage_EscapesNote pins that an operator-supplied note is
// HTML-escaped (html/template), so the page can't be turned into an XSS
// vector by note content.
func TestRenderPage_EscapesNote(t *testing.T) {
	html, err := RenderPage(PageData{Label: "Alive", Class: "alive", Note: `<script>alert(1)</script>`, FallbackURL: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("operator note was not HTML-escaped")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("expected the note to be escaped")
	}
}
