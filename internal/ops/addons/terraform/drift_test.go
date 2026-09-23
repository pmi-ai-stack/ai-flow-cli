package terraform_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func devConfig() *config.Config {
	return &config.Config{Ops: config.Ops{Environments: map[string]config.Environment{
		"dev": {Tier: "nonprod", Terraform: &config.TerraformBinding{Workspace: "dev", VarFile: "envs/dev.tfvars"}},
	}}}
}

// unconfiguredEnvConfig is an environment with no terraform binding at
// all — devConfig's sibling, shared by every "*.UnconfiguredEnv*" test
// in this package (Drift's and DryRun's own).
func unconfiguredEnvConfig() *config.Config {
	return &config.Config{Ops: config.Ops{Environments: map[string]config.Environment{"staging": {Tier: "nonprod"}}}}
}

// mockLiveInit registers the real init + workspace-select calls
// liveInit always makes before any plan, for devConfig's own "dev"
// workspace — shared by every Drift test below that gets past
// resolveBinding.
func mockLiveInit(x *exectest.Scripted) {
	x.On("terraform", []string{"init", "-input=false", "-no-color"}, exec.Output{})
	x.On("terraform", []string{"workspace", "select", "dev"}, exec.Output{})
}

func TestDriftNoneOnCleanExit(t *testing.T) {
	dir := t.TempDir()
	x := exectest.New(t)
	mockLiveInit(x)
	x.On("terraform", []string{"plan", "-detailed-exitcode", "-out=.ai-flow-drift.tfplan", "-input=false", "-no-color", "-var-file=envs/dev.tfvars"}, exec.Output{})

	a := terraform.New(x, devConfig(), "")
	report, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "dev", Tier: "nonprod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftNone {
		t.Errorf("Drift = %q, want none", report.Drift)
	}
}

func TestDriftPresentOnExitCode2(t *testing.T) {
	plan, err := os.ReadFile("testdata/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	x := exectest.New(t)
	mockLiveInit(x)
	x.OnFailWithOutput("terraform",
		[]string{"plan", "-detailed-exitcode", "-out=.ai-flow-drift.tfplan", "-input=false", "-no-color", "-var-file=envs/dev.tfvars"},
		exec.Output{ExitCode: 2},
		&envelope.Error{Code: "tool_failed", Message: "changes present"},
	)
	x.On("terraform", []string{"show", "-json", ".ai-flow-drift.tfplan"}, exec.Output{Stdout: string(plan)})

	a := terraform.New(x, devConfig(), "")
	report, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "dev", Tier: "nonprod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftPresent {
		t.Fatalf("Drift = %q, want present", report.Drift)
	}
	// Every non-noop, non-data resource_changes[] address from the
	// fixture: create+update+delete+replace = 4 addresses.
	if len(report.Resources) != 4 {
		t.Errorf("Resources = %v, want 4 (noop and the data source excluded)", report.Resources)
	}
}

func TestDriftRealFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	x := exectest.New(t)
	mockLiveInit(x)
	x.OnFailWithOutput("terraform",
		[]string{"plan", "-detailed-exitcode", "-out=.ai-flow-drift.tfplan", "-input=false", "-no-color", "-var-file=envs/dev.tfvars"},
		exec.Output{ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "real error"},
	)

	a := terraform.New(x, devConfig(), "")
	_, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "dev", Tier: "nonprod"}, &ops.Rendered{Dir: dir})
	if err == nil {
		t.Fatal("Drift() err = nil, want the real exit-1 failure propagated")
	}
}

func TestDriftUnboundEnvIsLiveCredentialsMissingWithNoCalls(t *testing.T) {
	x := exectest.New(t)
	a := terraform.New(x, devConfig(), "")
	_, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "", Tier: "nonprod"}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("err = %v, want ops.ErrLiveCredentialsMissing", err)
	}
	if len(x.Calls) != 0 {
		t.Errorf("Calls = %v, want none (ErrLiveCredentialsMissing must fire before any subprocess)", x.Calls)
	}
}

func TestDriftUnconfiguredEnvIsLiveCredentialsMissing(t *testing.T) {
	x := exectest.New(t)
	a := terraform.New(x, unconfiguredEnvConfig(), "")
	_, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "staging", Tier: "nonprod"}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("err = %v, want ops.ErrLiveCredentialsMissing (staging has no terraform binding)", err)
	}
}
