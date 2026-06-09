package main

// Config holds the evaluator's tunable knobs (sourced from env vars in
// main). The persisted dead-man-switch state lives in the shared
// internal/dmstate package.
type Config struct {
	// ThresholdFailsDays: a sign of life older than this trips the
	// switch (used by the alive predicate, not by Transition).
	ThresholdFailsDays int
	// WaitingPeriodDays: how long the waiting-period runs before
	// promoting to trigger-ready.
	WaitingPeriodDays int
}

// DefaultConfig is the issue-specified default: 30-day threshold,
// 30-day waiting-period.
func DefaultConfig() Config {
	return Config{ThresholdFailsDays: 30, WaitingPeriodDays: 30}
}
