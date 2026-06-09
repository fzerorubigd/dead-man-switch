package main

import (
	"strings"
	"testing"
	"time"
)

func TestSummarizeSignal(t *testing.T) {
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	d := func(daysAgo int) time.Time { return now.AddDate(0, 0, -daysAgo) }

	runs := []signalRun{
		{Conclusion: "success", At: d(2)},
		{Conclusion: "failure", At: d(3)},
		{Conclusion: "success", At: d(10)},
		{Conclusion: "cancelled", At: d(5)},
		{Conclusion: "failure", At: d(40)}, // outside 30d window
		{Conclusion: "success", At: d(50)}, // outside window, but newest success is d(2)
	}
	s := summarizeSignal("idlerpg", runs, now, 30)
	if s.Name != "idlerpg" {
		t.Errorf("name=%q", s.Name)
	}
	if s.Pass != 2 || s.Fail != 1 || s.Other != 1 {
		t.Errorf("counts: pass=%d fail=%d other=%d, want 2/1/1", s.Pass, s.Fail, s.Other)
	}
	if !strings.HasPrefix(s.LastSuccess, "2026-06-28") { // d(2)
		t.Errorf("lastSuccess=%q, want the d-2 success", s.LastSuccess)
	}
}

func TestSummarizeSignal_NeverSucceeded(t *testing.T) {
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	s := summarizeSignal("failed", []signalRun{{Conclusion: "failure", At: now}}, now, 30)
	if s.LastSuccess != "never" {
		t.Errorf("lastSuccess=%q, want never", s.LastSuccess)
	}
	if s.Fail != 1 || s.Pass != 0 {
		t.Errorf("counts pass=%d fail=%d", s.Pass, s.Fail)
	}
}

func TestRenderDigest(t *testing.T) {
	body, err := RenderDigest(DigestData{
		Status:         "waiting",
		LastSignOfLife: "Mon, 01 Jun 2026 00:00:00 UTC",
		WindowDays:     30,
		Signals: []SignalSummary{
			{Name: "IdleRPG", Pass: 20, Fail: 2, LastSuccess: "2026-06-28T00:00:00Z"},
			{Name: "Mostodon", Pass: 0, Fail: 30, Other: 1, LastSuccess: "never"},
		},
		GeneratedAt: "now",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"keep-alive digest",
		"Status: waiting",
		"last 30 days",
		"IdleRPG: 20 pass / 2 fail",
		"Mostodon: 0 pass / 30 fail / 1 other",
		"3+ weeks",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("digest missing %q in:\n%s", want, body)
		}
	}
}

func TestRenderDigest_NoSignals(t *testing.T) {
	body, err := RenderDigest(DigestData{Status: "alive", WindowDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "no signals found") {
		t.Errorf("expected the empty-signals line, got:\n%s", body)
	}
}
