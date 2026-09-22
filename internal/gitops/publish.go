package gitops

import (
	"context"
	"strings"
)

// IsDirty reports whether dir has any uncommitted change (staged,
// unstaged or untracked) — the read-only check both Commit and a
// `--simulate` preview use, so simulate never needs to guess.
func (g *GitOps) IsDirty(ctx context.Context, dir string) (bool, error) {
	out, err := g.run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out.Stdout) != "", nil
}

// Commit stages and commits every change in dir with message, unless
// the tree is already clean (design decision 11) — in which case it
// returns committed=false without calling `git add`/`git commit` at
// all, rather than parsing git's own "nothing to commit" failure text.
func (g *GitOps) Commit(ctx context.Context, dir, message string) (committed bool, err error) {
	dirty, err := g.IsDirty(ctx, dir)
	if err != nil {
		return false, err
	}
	if !dirty {
		return false, nil
	}
	if _, err := g.run(ctx, dir, "add", "-A"); err != nil {
		return false, err
	}
	if _, err := g.run(ctx, dir, "commit", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// Push pushes branch to origin, setting the upstream — harmless to
// repeat on a branch that already has one.
func (g *GitOps) Push(ctx context.Context, dir, branch string) error {
	_, err := g.run(ctx, dir, "push", "-u", "origin", branch)
	return err
}
