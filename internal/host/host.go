// Package host is the git-host interface pr create drives — the same
// one-interface-one-provider-today shape internal/tracker used for
// Jira (phase 4): a new host is a new package plus a config value,
// never a change to calling code.
package host

import "context"

// PullRequest is one repository's PR record: what ship-results.json's
// pull_requests[] entries and pr create's result both carry.
type PullRequest struct {
	Repo   string
	Number int
	URL    string
	Draft  bool
	// Reused reports whether an existing open PR was found and
	// returned instead of a new one being created — "reuses an open
	// PR on re-run" (the command catalog's own words for pr create).
	Reused bool
}

// Host is the git host a repository's PR lives on.
type Host interface {
	// EnsurePR finds an open PR with head=branch against base and
	// returns it (Reused: true), or creates one titled title with body
	// body, draft per draft, and returns that (Reused: false).
	EnsurePR(ctx context.Context, owner, repo, base, branch, title, body string, draft bool) (*PullRequest, error)
}
