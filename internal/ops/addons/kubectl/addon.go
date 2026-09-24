package kubectl

import (
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Addon is the real ops.Addon implementation for a directory of raw
// Kubernetes manifests. x is every subprocess call's own Execer; cfg
// supplies config.ops.environments[name].kube bindings (ops.Env itself
// carries only Name and Tier — the frozen Addon interface, phase 18
// on — so an addon that needs more must hold config itself, exactly
// terraform's own Addon shape, phase 21). Unlike terraform, this addon
// needs no home directory of its own (no call it makes needs a cache
// directory the way terraform's TF_PLUGIN_CACHE_DIR does), so New
// takes no home parameter — a deliberate, documented divergence from
// terraform.New's three-parameter shape (.ai/phase-22/implementation-log.md),
// not an oversight.
type Addon struct {
	x   exec.Execer
	cfg *config.Config
}

var _ ops.Addon = (*Addon)(nil)

// New builds the kubectl addon.
func New(x exec.Execer, cfg *config.Config) *Addon {
	return &Addon{x: x, cfg: cfg}
}

func (a *Addon) Name() string { return "kubectl" }

// Capabilities: every capability-matrix column for kubectl is filled
// except Unit harness ("—" — only e2e harnesses, kuttl/chainsaw on a
// kind cluster, apply here).
func (a *Addon) Capabilities() ops.Capabilities {
	return ops.Capabilities{
		Render: true, Validate: true, Drift: true, DryRun: true,
		Rollback: true, UnitTest: false, E2ETest: true,
	}
}

// timeout is every real subprocess call's own ops.tool_timeout_minutes
// budget — the same config field every other addon's calls already
// use.
func (a *Addon) timeout() time.Duration {
	return a.cfg.Ops.ToolTimeout()
}
