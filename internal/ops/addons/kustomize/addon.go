package kustomize

import (
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Addon is the real ops.Addon implementation for a Kustomize overlay.
// x is every subprocess call's own Execer; cfg supplies
// config.ops.environments[name].kube bindings — the same binding type
// kubectl's own addon uses (config.KubeBinding's own doc comment
// already reads "an environment's kubectl/helm/kustomize binding").
// No home parameter, mirroring kubectl.New's own two-parameter shape:
// this addon needs no cache directory of its own.
type Addon struct {
	x   exec.Execer
	cfg *config.Config
}

var _ ops.Addon = (*Addon)(nil)

// New builds the kustomize addon.
func New(x exec.Execer, cfg *config.Config) *Addon {
	return &Addon{x: x, cfg: cfg}
}

func (a *Addon) Name() string { return "kustomize" }

// Capabilities: every capability-matrix column for kustomize is filled
// except Unit harness ("—" — the matrix's own kustomize row is
// "harnesses as kubectl," and kubectl's own Unit harness is "—" too).
func (a *Addon) Capabilities() ops.Capabilities {
	return ops.Capabilities{
		Render: true, Validate: true, Drift: true, DryRun: true,
		Rollback: true, UnitTest: false, E2ETest: true,
	}
}

// timeout is every real subprocess call's own ops.tool_timeout_minutes
// budget.
func (a *Addon) timeout() time.Duration {
	return a.cfg.Ops.ToolTimeout()
}
