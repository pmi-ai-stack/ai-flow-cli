package githubactions_test

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
)

func devConfig() *config.Config {
	return &config.Config{Ops: config.Ops{ToolTimeoutMinutes: 5}}
}

func TestNameAndCapabilities(t *testing.T) {
	a := githubactions.New(nil, devConfig())
	if a.Name() != "github-actions" {
		t.Errorf("Name() = %q, want github-actions", a.Name())
	}
	caps := a.Capabilities()
	if !caps.Render || !caps.Validate || !caps.DryRun || !caps.Rollback || !caps.UnitTest || !caps.E2ETest {
		t.Errorf("Capabilities() = %+v, want every column but Drift true", caps)
	}
	if caps.Drift {
		t.Error("Capabilities().Drift = true, want false (not_applicable — no live counterpart)")
	}
}
