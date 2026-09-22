package standards

import "time"

// defaultClock is what Sync and StatusOf fall back to when their own
// Now parameter is nil. Isolated to this one file so the forbidigo
// "inject a clock" rule has exactly one literal time.Now to exclude,
// matching internal/handoff/clock.go's own precedent for the same
// shape.
var defaultClock = time.Now
