package main

import (
	"time"

	"github.com/fzerorubigd/life-tracker/internal/dmstate"
)

// Transition advances the dead-man-switch state machine. It is pure: no
// IO, no wall clock of its own — callers pass `now` and whether a recent
// sign of life exists (`alive`, computed by the alive predicate from the
// signal run history). `reset` is the operator escape hatch
// (workflow_dispatch reset=true), forcing the state back to alive.
//
// Transitions:
//
//	any state + reset            -> alive (TrippedAt cleared)
//	any state + alive            -> alive (recovery resets, even mid-wait)
//	alive     + !alive           -> waiting (TrippedAt = now)   [just tripped]
//	waiting   + !alive + elapsed -> trigger_ready               [period over]
//	waiting   + !alive + within  -> waiting (unchanged)
//	trigger_ready + !alive       -> trigger_ready (latched until reset)
func Transition(cur dmstate.State, alive bool, reset bool, now time.Time, cfg Config) dmstate.State {
	next := cur
	next.UpdatedAt = now

	switch {
	case reset:
		next.Status = dmstate.StatusAlive
		next.TrippedAt = nil

	case alive:
		// Any sign of life resets to alive, including during the
		// waiting-period (the issue's "signal recovered during wait").
		next.Status = dmstate.StatusAlive
		next.TrippedAt = nil

	default: // not alive, no reset
		switch cur.Status {
		case dmstate.StatusAlive, "":
			// Just tripped: start the waiting-period.
			t := now
			next.Status = dmstate.StatusWaiting
			next.TrippedAt = &t
		case dmstate.StatusWaiting:
			if cur.TrippedAt != nil && !now.Before(cur.TrippedAt.Add(waitingPeriod(cfg))) {
				next.Status = dmstate.StatusTriggerReady
			}
			// else: still within the waiting-period, stay waiting.
		case dmstate.StatusTriggerReady:
			// Latched; only an explicit reset (or a sign of life) leaves
			// this state.
		}
	}

	return next
}

func waitingPeriod(cfg Config) time.Duration {
	return time.Duration(cfg.WaitingPeriodDays) * 24 * time.Hour
}
