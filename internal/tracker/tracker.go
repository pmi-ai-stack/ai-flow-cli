// Package tracker defines the interface every issue-tracker provider
// implements. Reachable only through Tracker (03-command-layer.md,
// Boundaries) — a new provider is a new package plus a config value,
// never a change to calling code.
package tracker

import "context"

// StatusClass is one of the three tracker-status classes ai-flow
// reasons about, mapped to a provider's own status names by
// config.tracker.statuses. Todo only ever appears as a Transition
// source in the spec's "Ticket side effects" table — it is never a
// Transition target — but the constant exists so callers can name a
// ticket's starting class without a bare string.
type StatusClass string

const (
	StatusClassTodo       StatusClass = "todo"
	StatusClassInProgress StatusClass = "in_progress"
	StatusClassInReview   StatusClass = "in_review"
)

// Comment is one tracker comment, already provider-normalized.
type Comment struct {
	Author  string
	Body    string
	Created string
}

// Attachment is one tracker attachment's metadata: enough to name and
// fetch it, nothing provider-specific (no internal ids beyond what a
// re-fetch needs).
type Attachment struct {
	ID       string
	Filename string
	URL      string
}

// Issue is a fetched tracker issue, already provider-normalized —
// exactly the fields ticket.json needs, before the CLI layer computes
// branch_name (a pure function of IssueType and config.tracker.bug_types,
// so it does not belong on the provider-facing shape).
type Issue struct {
	Key         string
	Project     string
	IssueType   string
	Summary     string
	Description string
	Status      string
	Comments    []Comment
	Attachments []Attachment
}

// Tracker is the interface every issue-tracker provider implements.
type Tracker interface {
	// Fetch retrieves the current state of the issue at key.
	Fetch(ctx context.Context, key string) (*Issue, error)
	// Transition moves the issue at key to the provider's configured
	// status name for class, doing nothing when it is already there.
	// class is never StatusClassTodo — todo is only ever a source
	// status, never a transition target.
	Transition(ctx context.Context, key string, class StatusClass) error
	// Comment posts a new comment, or — when marker is non-empty —
	// edits the existing comment already carrying that marker instead
	// of creating a duplicate.
	Comment(ctx context.Context, key, marker, body string) error
	// DownloadAttachments fetches every one of issue's attachments into
	// dir, returning the filenames written, in issue.Attachments'
	// order.
	DownloadAttachments(ctx context.Context, issue *Issue, dir string) ([]string, error)
	// Reachable performs the cheapest authenticated call the provider
	// offers, to prove the configured credentials actually work.
	// doctor's own need: it has no ticket key to scope a real Fetch to
	// (doctor is one of the spec's commands that take no key).
	Reachable(ctx context.Context) error
	// Statuses returns every status name configured across the
	// tracker's own workflow(s) — doctor's source of truth for
	// verifying config.tracker.statuses' names actually exist there
	// (the inherited question phase 4 left for this phase to resolve).
	Statuses(ctx context.Context) ([]string, error)
}
