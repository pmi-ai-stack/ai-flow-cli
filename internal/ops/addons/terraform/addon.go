package terraform

import (
	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Addon is the real ops.Addon implementation for Terraform. x is every
// subprocess call's own Execer (the allowlist-matching, redacting
// production Runner in the shipped binary, exectest.Scripted in every
// test in this package); cfg supplies config.ops.environments[name].terraform
// bindings (workspace, var file — ops.Env itself carries only Name and
// Tier, so an addon that needs more must hold config itself, the same
// shape internal/ops/envs.Bind already established one layer up); home
// is ~/.ai-flow, needed only for TF_PLUGIN_CACHE_DIR (Render's own
// caller).
type Addon struct {
	x    exec.Execer
	cfg  *config.Config
	home string
}

var _ ops.Addon = (*Addon)(nil)

// New builds the terraform addon.
func New(x exec.Execer, cfg *config.Config, home string) *Addon {
	return &Addon{x: x, cfg: cfg, home: home}
}

func (a *Addon) Name() string { return "terraform" }

// Capabilities: every column of the capability matrix's own terraform
// row is filled.
func (a *Addon) Capabilities() ops.Capabilities {
	return ops.Capabilities{
		Render: true, Validate: true, Drift: true, DryRun: true,
		Rollback: true, UnitTest: true, E2ETest: true,
	}
}
