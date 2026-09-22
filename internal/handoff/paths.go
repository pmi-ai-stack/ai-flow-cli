package handoff

import "path/filepath"

// TicketDir returns ~/.ai-flow/<key> (home is whatever --home resolved
// to; every "~/.ai-flow/…" path in the spec means that value).
func TicketDir(home, key string) string {
	return filepath.Join(home, key)
}

// StatusPath returns a ticket's status.json path.
func StatusPath(home, key string) string {
	return filepath.Join(TicketDir(home, key), "status.json")
}

// RepoPath returns a ticket's repo.json path — written by `repo
// locate`/`repo confirm` and updated in place by `repo scope`.
func RepoPath(home, key string) string {
	return filepath.Join(TicketDir(home, key), "repo.json")
}

// TicketEventsPath returns a ticket's own events.jsonl path.
func TicketEventsPath(home, key string) string {
	return filepath.Join(TicketDir(home, key), "events.jsonl")
}

// ActivePath returns ~/.ai-flow/active.json — singular, system-wide,
// not per ticket (see the plan's design decision 7).
func ActivePath(home string) string {
	return filepath.Join(home, "active.json")
}

// GlobalEventsPath returns ~/.ai-flow/events.jsonl, for events that
// belong to no ticket — session starts, guard decisions outside any
// active ticket, doctor probes (phase 9 onward).
func GlobalEventsPath(home string) string {
	return filepath.Join(home, "events.jsonl")
}
