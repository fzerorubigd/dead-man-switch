package main

import (
	"testing"
	"time"

	"github.com/fzerorubigd/dead-man-switch/internal/dmstate"
)

func ptr(t time.Time) *time.Time { return &t }

func TestTransition(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig() // 30-day threshold, 30-day waiting-period

	cases := []struct {
		name        string
		cur         dmstate.State
		alive       bool
		reset       bool
		wantStatus  dmstate.Status
		wantTripped bool // whether TrippedAt should be non-nil after
	}{
		{
			name:        "not-tripped: alive stays alive",
			cur:         dmstate.State{Status: dmstate.StatusAlive},
			alive:       true,
			wantStatus:  dmstate.StatusAlive,
			wantTripped: false,
		},
		{
			name:        "just-tripped: alive -> waiting on loss of signal",
			cur:         dmstate.State{Status: dmstate.StatusAlive},
			alive:       false,
			wantStatus:  dmstate.StatusWaiting,
			wantTripped: true,
		},
		{
			name:        "in-waiting-period: still no signal, within period",
			cur:         dmstate.State{Status: dmstate.StatusWaiting, TrippedAt: ptr(now.AddDate(0, 0, -10))},
			alive:       false,
			wantStatus:  dmstate.StatusWaiting,
			wantTripped: true,
		},
		{
			name:        "signal-recovered-during-wait: resets to alive",
			cur:         dmstate.State{Status: dmstate.StatusWaiting, TrippedAt: ptr(now.AddDate(0, 0, -10))},
			alive:       true,
			wantStatus:  dmstate.StatusAlive,
			wantTripped: false,
		},
		{
			name:        "trigger-ready: waiting-period elapsed",
			cur:         dmstate.State{Status: dmstate.StatusWaiting, TrippedAt: ptr(now.AddDate(0, 0, -31))},
			alive:       false,
			wantStatus:  dmstate.StatusTriggerReady,
			wantTripped: true,
		},
		{
			name:        "trigger-ready boundary: exactly 30 days elapsed",
			cur:         dmstate.State{Status: dmstate.StatusWaiting, TrippedAt: ptr(now.AddDate(0, 0, -30))},
			alive:       false,
			wantStatus:  dmstate.StatusTriggerReady,
			wantTripped: true,
		},
		{
			name:        "trigger-ready latches while no signal",
			cur:         dmstate.State{Status: dmstate.StatusTriggerReady, TrippedAt: ptr(now.AddDate(0, 0, -60))},
			alive:       false,
			wantStatus:  dmstate.StatusTriggerReady,
			wantTripped: true,
		},
		{
			name:        "trigger-ready clears on recovery",
			cur:         dmstate.State{Status: dmstate.StatusTriggerReady, TrippedAt: ptr(now.AddDate(0, 0, -60))},
			alive:       true,
			wantStatus:  dmstate.StatusAlive,
			wantTripped: false,
		},
		{
			name:        "reset escape hatch from waiting",
			cur:         dmstate.State{Status: dmstate.StatusWaiting, TrippedAt: ptr(now.AddDate(0, 0, -10))},
			alive:       false,
			reset:       true,
			wantStatus:  dmstate.StatusAlive,
			wantTripped: false,
		},
		{
			name:        "reset escape hatch from trigger-ready",
			cur:         dmstate.State{Status: dmstate.StatusTriggerReady, TrippedAt: ptr(now.AddDate(0, 0, -60))},
			alive:       false,
			reset:       true,
			wantStatus:  dmstate.StatusAlive,
			wantTripped: false,
		},
		{
			name:        "empty state with no signal trips",
			cur:         dmstate.State{},
			alive:       false,
			wantStatus:  dmstate.StatusWaiting,
			wantTripped: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Transition(tc.cur, tc.alive, tc.reset, now, cfg)
			if got.Status != tc.wantStatus {
				t.Errorf("status: got %q want %q", got.Status, tc.wantStatus)
			}
			if (got.TrippedAt != nil) != tc.wantTripped {
				t.Errorf("trippedAt non-nil: got %v want %v", got.TrippedAt != nil, tc.wantTripped)
			}
			if !got.UpdatedAt.Equal(now) {
				t.Errorf("updatedAt: got %v want %v", got.UpdatedAt, now)
			}
		})
	}
}

// TestTransition_JustTrippedStampsNow pins that a fresh trip records the
// waiting-period start at the evaluation time.
func TestTransition_JustTrippedStampsNow(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	got := Transition(dmstate.State{Status: dmstate.StatusAlive}, false, false, now, DefaultConfig())
	if got.TrippedAt == nil || !got.TrippedAt.Equal(now) {
		t.Fatalf("TrippedAt: got %v want %v", got.TrippedAt, now)
	}
}

// TestTransition_WaitingPreservesTrippedAt pins that staying in the
// waiting-period does not move the original trip timestamp.
func TestTransition_WaitingPreservesTrippedAt(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	tripped := now.AddDate(0, 0, -5)
	got := Transition(dmstate.State{Status: dmstate.StatusWaiting, TrippedAt: ptr(tripped)}, false, false, now, DefaultConfig())
	if got.TrippedAt == nil || !got.TrippedAt.Equal(tripped) {
		t.Fatalf("TrippedAt moved: got %v want %v", got.TrippedAt, tripped)
	}
}
