package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// PrepareInput is Prepare's input: one repository's local checkout,
// the configured base branch, the ticket's branch name, whether to use
// a worktree (defaults.worktree), and where the merge-base worktree
// lives (Home/Key/RepoName build its path via BaseWorktreePath).
type PrepareInput struct {
	LocalPath string
	Base      string
	Branch    string
	Worktree  bool
	Home      string
	Key       string
	RepoName  string
}

// PrepareResult is what Prepare resolved: WorkingPath is where every
// later command (checkout-base, diff, commit, push) should operate —
// LocalPath unchanged when Worktree is false, the new worktree's path
// otherwise (design decision 8). BranchCreated is false on an
// idempotent re-run that found the branch/worktree already there.
type PrepareResult struct {
	WorkingPath      string
	BaseWorktreePath string
	BranchCreated    bool
}

// Prepare implements `git prepare KEY`'s per-repository work: refuse a
// dirty tree or a diverged base (design decision 7), branch from
// origin/<base> — worktree or plain checkout (design decision 8) —
// then create the merge-base worktree (design decision 9). Every step
// is safe to re-run: an already-clean fetch, an already-existing
// branch/worktree and an already-existing base worktree are all
// no-ops, not errors.
func (g *GitOps) Prepare(ctx context.Context, in PrepareInput) (PrepareResult, error) {
	statusOut, err := g.run(ctx, in.LocalPath, "status", "--porcelain")
	if err != nil {
		return PrepareResult{}, err
	}
	if strings.TrimSpace(statusOut.Stdout) != "" {
		return PrepareResult{}, dirtyWorktreeError(in.LocalPath)
	}

	if _, err := g.run(ctx, in.LocalPath, "fetch", "origin", in.Base); err != nil {
		return PrepareResult{}, err
	}

	if _, ok := g.tryRun(ctx, in.LocalPath, "rev-parse", "--verify", in.Base); ok {
		ahead, err := g.revListCount(ctx, in.LocalPath, "origin/"+in.Base, in.Base)
		if err != nil {
			return PrepareResult{}, err
		}
		if ahead > 0 {
			return PrepareResult{}, baseDivergedError(in.LocalPath, in.Base)
		}
	}

	workingPath := in.LocalPath
	branchCreated := false
	if in.Worktree {
		wtPath := siblingWorktreePath(in.LocalPath, in.RepoName, in.Key)
		if !dirExists(wtPath) {
			if _, err := g.run(ctx, in.LocalPath, "worktree", "add", "-b", in.Branch, wtPath, "origin/"+in.Base); err != nil {
				return PrepareResult{}, err
			}
			branchCreated = true
		}
		workingPath = wtPath
	} else {
		if _, ok := g.tryRun(ctx, in.LocalPath, "rev-parse", "--verify", in.Branch); !ok {
			if _, err := g.run(ctx, in.LocalPath, "checkout", "-b", in.Branch, "origin/"+in.Base); err != nil {
				return PrepareResult{}, err
			}
			branchCreated = true
		}
	}

	baseWT := BaseWorktreePath(in.Home, in.Key, in.RepoName)
	if !dirExists(baseWT) {
		mergeBaseOut, err := g.run(ctx, in.LocalPath, "merge-base", "origin/"+in.Base, in.Branch)
		if err != nil {
			return PrepareResult{}, err
		}
		sha := strings.TrimSpace(mergeBaseOut.Stdout)
		if err := os.MkdirAll(filepath.Dir(baseWT), 0o700); err != nil {
			return PrepareResult{}, fmt.Errorf("gitops: create %s: %w", filepath.Dir(baseWT), err)
		}
		if _, err := g.run(ctx, in.LocalPath, "worktree", "add", "--detach", baseWT, sha); err != nil {
			return PrepareResult{}, err
		}
	}

	return PrepareResult{WorkingPath: workingPath, BaseWorktreePath: baseWT, BranchCreated: branchCreated}, nil
}

// revListCount runs `git rev-list --count from..to` and parses the
// result — the integer count of commits reachable from to but not
// from, used by the divergence check (design decision 7).
func (g *GitOps) revListCount(ctx context.Context, dir, from, to string) (int, error) {
	out, err := g.run(ctx, dir, "rev-list", "--count", from+".."+to)
	if err != nil {
		return 0, err
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(out.Stdout))
	if convErr != nil {
		return 0, fmt.Errorf("gitops: parse rev-list --count output %q: %w", out.Stdout, convErr)
	}
	return n, nil
}

// siblingWorktreePath is the deterministic worktree location design
// decision 8 names: a directory next to localPath, so a second
// `git prepare KEY` recognizes and reuses it instead of erroring or
// duplicating it.
func siblingWorktreePath(localPath, repoName, key string) string {
	return filepath.Join(filepath.Dir(localPath), repoName+"-"+key)
}

func dirtyWorktreeError(localPath string) *envelope.Error {
	return &envelope.Error{
		Code:    "dirty_worktree",
		Message: fmt.Sprintf("%s has uncommitted changes; commit, stash or discard them, then retry", localPath),
	}
}

func baseDivergedError(localPath, base string) *envelope.Error {
	return &envelope.Error{
		Code: "base_diverged",
		Message: fmt.Sprintf(
			"%s: the local %s branch has commits not on origin/%s; resolve locally, then retry",
			localPath, base, base,
		),
	}
}
