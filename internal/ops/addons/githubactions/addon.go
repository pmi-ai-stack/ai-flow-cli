package githubactions

import (
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Addon is the real ops.Addon implementation for GitHub Actions
// workflow files. x is every subprocess call's own Execer; cfg is
// held for symmetry with every other addon's own constructor shape
// (kubectl.New/helm.New/kustomize.New) even though this addon never
// reads a live binding from it — a workflow file has no live
// counterpart at all (the spec's own "unbound environment" case,
// confirmed by internal/ops/envs.Bind's own hasLiveCounterpart
// parameter, phase 27's own first false caller). No home parameter:
// this addon needs no cache directory of its own.
type Addon struct {
	x   exec.Execer
	cfg *config.Config
}

var _ ops.Addon = (*Addon)(nil)

// New builds the github-actions addon.
func New(x exec.Execer, cfg *config.Config) *Addon {
	return &Addon{x: x, cfg: cfg}
}

func (a *Addon) Name() string { return "github-actions" }

// Capabilities: the matrix's own github-actions row. Drift is false —
// not a method that always returns DriftNotApplicable and is never
// called (internal/cli/ops_drift.go's own driftOneTarget already
// defaults every target's report to DriftNotApplicable and only calls
// Addon.Drift when Capabilities().Drift is true, the phase-27 plan's
// own §1.1.1). UnitTest/E2ETest are true: both harness cells are
// detected today (Harnesses), even though neither is wired into
// internal/ops/harness.ToolFor yet — the same "detection is the
// capability" reading every prior addon phase already established.
func (a *Addon) Capabilities() ops.Capabilities {
	return ops.Capabilities{
		Render: true, Validate: true, DryRun: true, Rollback: true,
		UnitTest: true, E2ETest: true,
	}
}

func (a *Addon) timeout() time.Duration {
	return a.cfg.Ops.ToolTimeout()
}
