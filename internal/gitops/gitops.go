// Package gitops is every git operation the CLI performs: sync, branch,
// worktree, checkout, diff, commit, push, the merge-base worktree.
// Every call goes through an injected exec.Execer — this package never
// imports os/exec itself (internal/ops/exec is the only package
// allowed to) — so its own tests run against a fake, and the
// `-tags integration` suite runs the same code against a real git.
package gitops

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// GitOps drives git through exec. Timeout is applied to every
// subprocess this package runs (config.ops.tool_timeout_minutes, the
// same budget every other tool shares — see the spec's Execution
// allowlist section).
type GitOps struct {
	Exec    exec.Execer
	Timeout time.Duration
}

// New builds a GitOps.
func New(e exec.Execer, timeout time.Duration) *GitOps {
	return &GitOps{Exec: e, Timeout: timeout}
}

// BaseWorktreePath returns the fixed location of a ticket/repository's
// merge-base worktree: ~/.ai-flow/cache/basewt/<KEY>/<repo> — under the
// reserved "cache" name (never a handoff artifact), the same placement
// rule the rendering cache follows. Phase 19's `ops render --ref base`
// reads from here; this package only creates it (Prepare) and proves
// it sits at the right commit.
func BaseWorktreePath(home, key, repo string) string {
	return filepath.Join(home, "cache", "basewt", key, repo)
}

// run is the one call site every method in this package funnels
// through: git, in dir, with the shared timeout.
func (g *GitOps) run(ctx context.Context, dir string, args ...string) (exec.Output, error) {
	return g.Exec.Run(ctx, "git", args, exec.RunOpts{Dir: dir, Timeout: g.Timeout})
}

// tryRun is run for callers that treat a non-zero exit as an expected
// negative answer (e.g. "does this ref exist locally yet?") rather
// than a failure — ok reports whether the command exited zero.
func (g *GitOps) tryRun(ctx context.Context, dir string, args ...string) (exec.Output, bool) {
	out, err := g.run(ctx, dir, args...)
	return out, err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
