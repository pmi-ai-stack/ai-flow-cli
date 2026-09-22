package gitops

import (
	"context"
	"fmt"
	"strings"
)

// RemoteOwnerRepo parses dir's origin remote URL into (owner, repo) —
// design decision 12's source of truth for pr create, since every
// repository is already a real local clone with a configured origin
// and no config field maps a repo name to a git-host owner/repo slug.
func (g *GitOps) RemoteOwnerRepo(ctx context.Context, dir string) (owner, repo string, err error) {
	out, err := g.run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return "", "", err
	}
	return parseOwnerRepo(strings.TrimSpace(out.Stdout))
}

// parseOwnerRepo handles both URL shapes git accepts for a remote:
// the SCP-like shorthand ("git@host:owner/repo.git") and any URL form
// ("https://host/owner/repo.git", "ssh://git@host/owner/repo.git") —
// in both cases the last two "/"-separated path segments, .git
// trimmed, are the owner and repo.
func parseOwnerRepo(url string) (owner, repo string, err error) {
	trimmed := strings.TrimSuffix(url, ".git")

	if at := strings.Index(trimmed, "@"); at >= 0 && !strings.Contains(trimmed, "://") {
		// SCP-like shorthand: user@host:path — the host:path split is
		// the first ":" after the "@", not a "://" scheme separator.
		if colon := strings.Index(trimmed[at:], ":"); colon >= 0 {
			trimmed = trimmed[at+colon+1:]
		}
	}

	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("gitops: cannot parse owner/repo from remote URL %q", url)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}
