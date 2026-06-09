package main

import (
	"bytes"
	"text/template"
	"time"
)

// SignalSummary is one life-signal's pass/fail tally over the digest
// window plus its most recent success.
type SignalSummary struct {
	Name        string
	Pass        int
	Fail        int
	Other       int
	LastSuccess string
}

// DigestData is the keep-alive digest's view model.
type DigestData struct {
	Status         string
	LastSignOfLife string
	WindowDays     int
	Signals        []SignalSummary
	GeneratedAt    string
}

// signalRun is one workflow run (conclusion + time) used to compute a
// summary.
type signalRun struct {
	Conclusion string
	At         time.Time
}

// summarizeSignal tallies a signal's runs: pass/fail/other counts within
// the window ending at now, plus the most recent success across all runs.
func summarizeSignal(name string, runs []signalRun, now time.Time, windowDays int) SignalSummary {
	cutoff := now.AddDate(0, 0, -windowDays)
	s := SignalSummary{Name: name, LastSuccess: "never"}
	var lastSuccess time.Time
	for _, r := range runs {
		if r.Conclusion == "success" && r.At.After(lastSuccess) {
			lastSuccess = r.At
		}
		if r.At.Before(cutoff) {
			continue
		}
		switch r.Conclusion {
		case "success":
			s.Pass++
		case "failure":
			s.Fail++
		default:
			s.Other++
		}
	}
	if !lastSuccess.IsZero() {
		s.LastSuccess = lastSuccess.Format(time.RFC3339)
	}
	return s
}

var digestTmpl = template.Must(template.New("digest").Parse(digestText))

// RenderDigest renders the plain-text keep-alive digest body.
func RenderDigest(d DigestData) (string, error) {
	var buf bytes.Buffer
	if err := digestTmpl.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const digestText = `dead-man-switch keep-alive digest

Status: {{.Status}}
Last sign of life: {{.LastSignOfLife}}

Per-signal summary (last {{.WindowDays}} days):
{{range .Signals}}  - {{.Name}}: {{.Pass}} pass / {{.Fail}} fail{{if .Other}} / {{.Other}} other{{end}} (last success: {{.LastSuccess}})
{{else}}  (no signals found)
{{end}}
If you don't receive this digest for 3+ weeks, expect a death-trigger email.

Generated: {{.GeneratedAt}}
`
