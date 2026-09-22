package gitops

import (
	"context"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// DiffResult is one repository's diff against its base: the changed
// file paths (tracked changes and new untracked files alike) and the
// unified diff text (tracked changes only — the same thing a
// plain `git diff` shows a human; an untracked file has no diff to
// show until it is staged).
type DiffResult struct {
	Files []string
	Diff  string
}

// Diff computes dir's diff against origin/base using git's own
// triple-dot syntax (design decision 10) — it already resolves against
// the merge base, so no separate merge-base computation is needed here
// (only the base worktree, built by Prepare, needs an explicit SHA).
// `git diff --name-only` never lists untracked files (that is what
// "untracked" means to git), so Files also folds in
// `git ls-files --others --exclude-standard` — otherwise a new file
// the implementation added but never staged would silently vanish
// from repo scope's "in-scope files".
func (g *GitOps) Diff(ctx context.Context, dir, base string) (DiffResult, error) {
	ref := "origin/" + base + "...HEAD"

	trackedOut, err := g.run(ctx, dir, "diff", ref, "--name-only")
	if err != nil {
		return DiffResult{}, err
	}
	untrackedOut, err := g.run(ctx, dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return DiffResult{}, err
	}
	textOut, err := g.run(ctx, dir, "diff", ref)
	if err != nil {
		return DiffResult{}, err
	}

	files := dedupSortedLines(trackedOut.Stdout, untrackedOut.Stdout)
	return DiffResult{Files: files, Diff: textOut.Stdout}, nil
}

// dedupSortedLines splits each of blocks on "\n", drops empty lines,
// then dedups and sorts the result via strutil.DedupSorted — the
// byte-stable file-list shape every caller of Diff (repo scope, in
// particular) needs.
func dedupSortedLines(blocks ...string) []string {
	var lines []string
	for _, block := range blocks {
		for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
			if line != "" {
				lines = append(lines, line)
			}
		}
	}
	return strutil.DedupSorted(lines)
}
