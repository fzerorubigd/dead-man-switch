package main

import "time"

const conclusionSuccess = "success"

// SignalRun is one run of a life-signal workflow. Conclusion follows the
// GitHub Actions vocabulary ("success", "failure", ...). A successful run
// is a sign of life: each signal CLI exits 0 when it sees recent activity
// and non-zero ("are you dead?") otherwise.
type SignalRun struct {
	Conclusion string
	At         time.Time
}

// lastSignOfLife returns the time of the newest successful run across all
// signals (D2: last sign of life across ANY signal), or the zero time if
// no signal has ever succeeded.
func lastSignOfLife(runs []SignalRun) time.Time {
	var newest time.Time
	for _, r := range runs {
		if r.Conclusion == conclusionSuccess && r.At.After(newest) {
			newest = r.At
		}
	}
	return newest
}

// alive reports whether the newest sign of life falls within the
// threshold window ending at now. A zero last (no success ever seen) is
// not alive.
func alive(last, now time.Time, cfg Config) bool {
	if last.IsZero() {
		return false
	}
	return !last.Before(now.Add(-thresholdWindow(cfg)))
}

func thresholdWindow(cfg Config) time.Duration {
	return time.Duration(cfg.ThresholdFailsDays) * 24 * time.Hour
}
