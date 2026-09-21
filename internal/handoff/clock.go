package handoff

import "time"

// Now is the clock every writer in this package uses for a
// human-decision timestamp: a status.json approval's "at", and
// active.json's last_event_at. Tests override it (see
// handofftest.FixedClock) so identical inputs still produce identical
// bytes (07-determinism.md) — production code never calls time.Now
// directly outside events.go, which needs no such reproducibility
// since events.jsonl is append-only real time (.golangci.yml carries a
// forbidigo exclusion for both files).
var Now = time.Now
