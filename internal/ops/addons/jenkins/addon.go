package jenkins

import (
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Addon is the real ops.Addon implementation for Jenkins pipeline
// definitions. No home parameter — the same no-cache-directory shape
// kubectl.New/helm.New/kustomize.New already established.
type Addon struct {
	x   exec.Execer
	cfg *config.Config
}

var _ ops.Addon = (*Addon)(nil)

// New builds the jenkins addon.
func New(x exec.Execer, cfg *config.Config) *Addon {
	return &Addon{x: x, cfg: cfg}
}

func (a *Addon) Name() string { return "jenkins" }

// Capabilities: the matrix's own jenkins row — every column filled,
// matching terraform's/helm's "every column filled" shape.
func (a *Addon) Capabilities() ops.Capabilities {
	return ops.Capabilities{
		Render: true, Validate: true, Drift: true, DryRun: true,
		Rollback: true, UnitTest: true, E2ETest: true,
	}
}

func (a *Addon) timeout() time.Duration {
	return a.cfg.Ops.ToolTimeout()
}
