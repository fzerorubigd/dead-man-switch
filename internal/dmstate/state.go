// Package dmstate is the shared dead-man-switch state: the lifecycle
// status, the persisted State record, and its read/write on the orphan
// `state` branch. The aggregate evaluator writes it; the trigger layer
// reads it.
package dmstate

import "time"

// Status is the dead-man-switch lifecycle state.
type Status string

const (
	// StatusAlive: a sign of life was seen within the threshold window.
	StatusAlive Status = "alive"
	// StatusWaiting: the threshold tripped (no sign of life); the
	// waiting-period is running before the switch fires.
	StatusWaiting Status = "waiting"
	// StatusTriggerReady: the waiting-period elapsed with no recovery;
	// the trigger layer may now fire the death actions.
	StatusTriggerReady Status = "trigger_ready"
)

// State is the persisted dead-man-switch state — the single record the
// aggregate evaluator reads at the start of a run and writes at the end,
// and the record the trigger layer reads to gate firing.
type State struct {
	Status Status `json:"status"`
	// LastSignOfLife is the newest signal success observed, for the
	// status page; nil if no success has ever been seen.
	LastSignOfLife *time.Time `json:"last_sign_of_life,omitempty"`
	// TrippedAt is when Status entered StatusWaiting (the waiting-period
	// start); nil while alive.
	TrippedAt *time.Time `json:"tripped_at,omitempty"`
	// UpdatedAt is when this state was last evaluated.
	UpdatedAt time.Time `json:"updated_at"`
}
