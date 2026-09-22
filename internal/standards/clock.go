package standards

import "time"

// defaultClock is what Sync and StatusOf fall back to when their own
// Now parameter is nil. Isolated to this one file so the forbidigo
// "inject a clock" rule has exactly one literal time.Now to exclude.
// Unlike internal/handoff.Now — an exported var tests reassign directly
// (handofftest.FixedClock) — this one is never reassigned: every caller
// injects its own clock through an explicit Now parameter instead, and
// defaultClock only fills in when that parameter is nil.
var defaultClock = time.Now

// clockOrDefault returns now, or defaultClock when now is nil — the
// one place Sync and StatusOf's identical nil-fallback lived twice.
func clockOrDefault(now func() time.Time) func() time.Time {
	if now == nil {
		return defaultClock
	}
	return now
}
