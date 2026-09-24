package kustomize

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

func TestNameAndCapabilities(t *testing.T) {
	a := New(nil, &config.Config{})
	if a.Name() != "kustomize" {
		t.Errorf("Name() = %q, want kustomize", a.Name())
	}
	c := a.Capabilities()
	if !c.Render || !c.Validate || !c.Drift || !c.DryRun || !c.Rollback || !c.E2ETest {
		t.Errorf("Capabilities() = %+v, want every one true except UnitTest", c)
	}
	if c.UnitTest {
		t.Error("Capabilities().UnitTest = true, want false — the capability matrix's own kustomize row has no unit harness")
	}
}
