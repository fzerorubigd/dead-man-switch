package dmstate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestState_JSONRoundTrip(t *testing.T) {
	tripped := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	life := time.Date(2026, 5, 20, 8, 30, 0, 0, time.UTC)
	in := State{
		Status:         StatusWaiting,
		LastSignOfLife: &life,
		TrippedAt:      &tripped,
		UpdatedAt:      time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out State
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != in.Status {
		t.Errorf("status: got %q want %q", out.Status, in.Status)
	}
	if out.TrippedAt == nil || !out.TrippedAt.Equal(tripped) {
		t.Errorf("trippedAt: got %v want %v", out.TrippedAt, tripped)
	}
	if out.LastSignOfLife == nil || !out.LastSignOfLife.Equal(life) {
		t.Errorf("lastSignOfLife: got %v want %v", out.LastSignOfLife, life)
	}
	if !out.UpdatedAt.Equal(in.UpdatedAt) {
		t.Errorf("updatedAt: got %v want %v", out.UpdatedAt, in.UpdatedAt)
	}
}

// TestState_OmitsEmptyTimestamps pins that nil pointers stay out of the
// JSON, so an alive state has no tripped_at / last_sign_of_life noise.
func TestState_OmitsEmptyTimestamps(t *testing.T) {
	b, err := json.Marshal(State{Status: StatusAlive, UpdatedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "tripped_at") {
		t.Errorf("expected tripped_at omitted, got %s", s)
	}
	if strings.Contains(s, "last_sign_of_life") {
		t.Errorf("expected last_sign_of_life omitted, got %s", s)
	}
}
