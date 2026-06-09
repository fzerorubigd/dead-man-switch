package main

import (
	"testing"
	"time"
)

func TestLastSignOfLife(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	d := func(days int) time.Time { return base.AddDate(0, 0, days) }

	t.Run("picks newest success, ignores failures", func(t *testing.T) {
		runs := []SignalRun{
			{Conclusion: "success", At: d(1)},
			{Conclusion: "failure", At: d(9)}, // newer but a failure
			{Conclusion: "success", At: d(5)},
			{Conclusion: "success", At: d(3)},
		}
		if got := lastSignOfLife(runs); !got.Equal(d(5)) {
			t.Fatalf("got %v want %v", got, d(5))
		}
	})

	t.Run("zero when no successes", func(t *testing.T) {
		runs := []SignalRun{
			{Conclusion: "failure", At: d(1)},
			{Conclusion: "cancelled", At: d(2)},
		}
		if got := lastSignOfLife(runs); !got.IsZero() {
			t.Fatalf("got %v want zero", got)
		}
	})

	t.Run("zero on empty", func(t *testing.T) {
		if got := lastSignOfLife(nil); !got.IsZero() {
			t.Fatalf("got %v want zero", got)
		}
	})
}

func TestAlive(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig() // 30-day threshold

	cases := []struct {
		name string
		last time.Time
		want bool
	}{
		{"recent sign of life is alive", now.AddDate(0, 0, -5), true},
		{"just inside the window", now.AddDate(0, 0, -29), true},
		{"exactly at the boundary is alive", now.Add(-thresholdWindow(cfg)), true},
		{"past the window is dead", now.AddDate(0, 0, -31), false},
		{"zero (never seen) is dead", time.Time{}, false},
		{"future timestamp is alive", now.AddDate(0, 0, 1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := alive(tc.last, now, cfg); got != tc.want {
				t.Fatalf("alive=%v want %v", got, tc.want)
			}
		})
	}
}
