package main

import (
	"bytes"
	"html/template"

	"github.com/fzerorubigd/life-tracker/internal/dmstate"
)

// PageData is the rendered status page's view model.
type PageData struct {
	Label          string // human status word (Alive / Waiting / Triggered / Unknown)
	Class          string // CSS class driving the colour
	Description    string // one-line explanation
	LastSignOfLife string // formatted timestamp, or "unknown"
	Note           string // optional operator note
	Domain         string // custom domain, if any (shown for posterity)
	FallbackURL    string // the GitHub-Pages default URL (survives a domain lapse)
	GeneratedAt    string // when this page was rendered
}

// statusView maps a dead-man-switch status to its page label, CSS class,
// and one-line description.
func statusView(s dmstate.Status) (label, class, desc string) {
	switch s {
	case dmstate.StatusAlive:
		return "Alive", "alive", "A life signal was seen recently."
	case dmstate.StatusWaiting:
		return "Waiting", "waiting", "No recent life signal — the waiting period is running."
	case dmstate.StatusTriggerReady:
		return "Triggered", "triggered", "The waiting period elapsed with no sign of life."
	default:
		return "Unknown", "unknown", "No status has been recorded yet."
	}
}

var pageTmpl = template.Must(template.New("status").Parse(pageHTML))

// RenderPage renders the self-contained status-page HTML. It is pure (no
// IO) so every status renders identically in tests and in CI.
func RenderPage(d PageData) (string, error) {
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// pageHTML is intentionally a single self-contained file (inline CSS, no
// external assets) so the page keeps working with no dependencies after
// the operator is gone.
const pageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>am I alive?</title>
<style>
  :root { color-scheme: light dark; }
  body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
         font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif; background: #11151a; color: #e7edf3; }
  .card { max-width: 36rem; padding: 2.5rem; text-align: center; }
  .status { font-size: clamp(2.5rem, 9vw, 5rem); font-weight: 800; letter-spacing: -0.02em; margin: 0 0 0.5rem; }
  .status.alive { color: #43d17a; }
  .status.waiting { color: #f5b73d; }
  .status.triggered { color: #ef5350; }
  .status.unknown { color: #9aa6b2; }
  .desc { font-size: 1.15rem; color: #c2ccd6; margin: 0 0 2rem; }
  .meta { font-size: 0.95rem; color: #8b97a3; line-height: 1.7; }
  .meta b { color: #c2ccd6; font-weight: 600; }
  .note { margin: 1.5rem 0; padding: 1rem 1.25rem; border-left: 3px solid #3a4750; background: #1a2027;
          text-align: left; border-radius: 4px; color: #c2ccd6; white-space: pre-wrap; }
  a { color: #6ab0ff; }
  .foot { margin-top: 2rem; font-size: 0.8rem; color: #6b7680; }
</style>
</head>
<body>
  <main class="card">
    <h1 class="status {{.Class}}">{{.Label}}</h1>
    <p class="desc">{{.Description}}</p>
    {{if .Note}}<div class="note">{{.Note}}</div>{{end}}
    <div class="meta">
      <div>Last life signal: <b>{{.LastSignOfLife}}</b></div>
      <div>Page generated: <b>{{.GeneratedAt}}</b></div>
      {{if .Domain}}<div>Primary address: <b>{{.Domain}}</b></div>{{end}}
    </div>
    <p class="foot">
      If this address ever stops resolving, this page also lives at
      <a href="{{.FallbackURL}}">{{.FallbackURL}}</a>.
    </p>
  </main>
</body>
</html>
`
