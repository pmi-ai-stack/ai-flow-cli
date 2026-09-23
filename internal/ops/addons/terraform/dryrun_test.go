package terraform_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestDryRunUnboundEnvIsLiveCredentialsMissingWithNoCalls(t *testing.T) {
	x := exectest.New(t)
	a := terraform.New(x, devConfig(), "")
	_, err := a.DryRun(context.Background(), ops.Target{}, ops.Env{Name: "", Tier: "nonprod"}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("err = %v, want ops.ErrLiveCredentialsMissing", err)
	}
	if len(x.Calls) != 0 {
		t.Errorf("Calls = %v, want none (an --offline call must never reach a subprocess)", x.Calls)
	}
}

func TestDryRunUnconfiguredEnvIsLiveCredentialsMissing(t *testing.T) {
	x := exectest.New(t)
	a := terraform.New(x, unconfiguredEnvConfig(), "")
	_, err := a.DryRun(context.Background(), ops.Target{}, ops.Env{Name: "staging", Tier: "nonprod"}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("err = %v, want ops.ErrLiveCredentialsMissing (staging has no terraform binding)", err)
	}
}
