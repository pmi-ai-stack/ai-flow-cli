package helm

import (
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Addon is the real ops.Addon implementation for a Helm chart. x is
// every subprocess call's own Execer; cfg supplies
// config.ops.environments[name].kube bindings — the same binding type
// kubectl/kustomize use (config.KubeBinding's own doc comment already
// reads "an environment's kubectl/helm/kustomize binding"; no separate
// HelmBinding config type exists — see .ai/phase-26/plan.md §0.6). No
// home parameter: this addon needs no cache directory of its own.
type Addon struct {
	x   exec.Execer
	cfg *config.Config
}

var _ ops.Addon = (*Addon)(nil)

// New builds the helm addon.
func New(x exec.Execer, cfg *config.Config) *Addon {
	return &Addon{x: x, cfg: cfg}
}

func (a *Addon) Name() string { return "helm" }

// Capabilities: every capability-matrix column for helm is filled —
// the matrix's own helm row has a real Unit harness ("helm unittest,
// ct lint"), unlike kubectl's/kustomize's own "—".
func (a *Addon) Capabilities() ops.Capabilities {
	return ops.Capabilities{
		Render: true, Validate: true, Drift: true, DryRun: true,
		Rollback: true, UnitTest: true, E2ETest: true,
	}
}

func (a *Addon) timeout() time.Duration {
	return a.cfg.Ops.ToolTimeout()
}
