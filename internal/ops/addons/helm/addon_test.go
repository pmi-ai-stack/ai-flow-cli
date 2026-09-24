package helm

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

func TestNameAndCapabilities(t *testing.T) {
	a := New(nil, &config.Config{})
	if a.Name() != "helm" {
		t.Errorf("Name() = %q, want helm", a.Name())
	}
	c := a.Capabilities()
	if !c.Render || !c.Validate || !c.Drift || !c.DryRun || !c.Rollback || !c.UnitTest || !c.E2ETest {
		t.Errorf("Capabilities() = %+v, want every one true — the capability matrix's own helm row has a real unit harness", c)
	}
}
