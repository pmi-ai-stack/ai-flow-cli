//go:build ops

package terraform_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// builtinModuleRepo stages testdata/module-builtin as infra/sandbox in
// a temporary repository: a root module whose only resource comes from
// terraform's built-in provider, with a local backend — so init, plan
// and show run for real with no network and no credentials.
func builtinModuleRepo(t *testing.T) (repoDir, moduleDir string) {
	t.Helper()
	repoDir = t.TempDir()
	moduleDir = filepath.Join(repoDir, "infra", "sandbox")
	if err := os.MkdirAll(moduleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for src, dst := range map[string]string{"main.tf.fixture": "main.tf", "sandbox.tfvars": "sandbox.tfvars"} {
		data, err := os.ReadFile(filepath.Join("testdata", "module-builtin", src))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(moduleDir, dst), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return repoDir, moduleDir
}

// sandboxConfig binds environment "sandbox" to the default workspace
// and the module's own var file — so Drift and DryRun pass
// `workspace select`, `-var-file=` and `-out=` through the real
// allowlist, the argv phase 54 found refused since phase 21.
func sandboxConfig() *config.Config {
	return &config.Config{Ops: config.Ops{
		ToolTimeoutMinutes: 5,
		Environments: map[string]config.Environment{
			"sandbox": {Tier: "sandbox", Terraform: &config.TerraformBinding{Workspace: "default", VarFile: "sandbox.tfvars"}},
		},
	}}
}

// TestRealTerraformDetectRenderValidate is the spec's own "-tags ops
// runs real terraform when it is on PATH" Done when clause, through the
// real Runner and therefore the real allowlist. Validate needs tflint
// as well; without it only that half is skipped.
func TestRealTerraformDetectRenderValidate(t *testing.T) {
	exectest.RequireTool(t, "terraform")
	repoDir, moduleDir := builtinModuleRepo(t)

	a := terraform.New(exec.NewRunner(), sandboxConfig(), t.TempDir())
	repo := detect.Repo{Name: "acme.infra", LocalPath: repoDir}

	targets, err := a.Detect(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly one root module", targets)
	}

	rendered, err := a.Render(context.Background(), ops.Target{Addon: "terraform", Path: moduleDir}, ops.RefBranch, t.TempDir())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if exectest.HasTool(t, "tflint") {
		if _, err := a.Validate(context.Background(), rendered); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	}

	harnesses, err := a.Harnesses(repo, targets[0])
	if err != nil {
		t.Fatalf("Harnesses: %v", err)
	}
	if len(harnesses) != 0 {
		t.Errorf("harnesses = %v, want none (the fixture module ships no .tftest.hcl)", harnesses)
	}
}

// TestRealTerraformDriftAndDryRun runs the two live stages for real.
// The local state is empty, so the declared terraform_data resource is
// both drift (live lacks it: plan -detailed-exitcode exits 2) and the
// dry run's one create.
func TestRealTerraformDriftAndDryRun(t *testing.T) {
	exectest.RequireTool(t, "terraform")
	_, moduleDir := builtinModuleRepo(t)

	a := terraform.New(exec.NewRunner(), sandboxConfig(), t.TempDir())
	target := ops.Target{Addon: "terraform", Repo: "acme.infra", Path: moduleDir}
	env := ops.Env{Name: "sandbox", Tier: "sandbox"}
	ctx := context.Background()

	rendered, err := a.Render(ctx, target, ops.RefBranch, t.TempDir())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	drift, err := a.Drift(ctx, target, env, rendered)
	if err != nil {
		t.Fatalf("Drift: %v", err)
	}
	if drift.Drift != ops.DriftPresent || !slices.Equal(drift.Resources, []string{"terraform_data.marker"}) {
		t.Errorf("Drift = %+v, want present with [terraform_data.marker]", drift)
	}

	cs, err := a.DryRun(ctx, target, env, rendered)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if len(cs.Actions) != 1 {
		t.Fatalf("DryRun actions = %+v, want exactly one", cs.Actions)
	}
	got := cs.Actions[0]
	if got.Action != ops.ActionCreate || got.Address != "terraform_data.marker" || got.Env != "sandbox" || got.Tier != "sandbox" {
		t.Errorf("DryRun action = %+v, want a create of terraform_data.marker in sandbox", got)
	}
	for _, f := range []string{".ai-flow-dryrun.tfplan", ".ai-flow-drift.tfplan"} {
		if _, err := os.Stat(filepath.Join(rendered.Dir, f)); !os.IsNotExist(err) {
			t.Errorf("plan file %s left in the rendering (stat err = %v)", f, err)
		}
	}
}
