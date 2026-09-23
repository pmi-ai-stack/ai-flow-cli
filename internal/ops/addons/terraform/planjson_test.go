package terraform_test

import (
	"context"
	"os"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// TestDryRunNormalizesRecordedPlan drives DryRun end to end against
// the recorded plan.json fixture through a Scripted execer — the
// plan-JSON normalizer's own primary proof, exercised through the
// public Addon method rather than an unexported helper directly, since
// buildChangeset/parsePlanJSON are internal to this package and this
// _test.go file lives in the external terraform_test package (the
// same convention internal/handoff/handofftest's own doc comment
// explains: an external test package cannot reach unexported names).
func TestDryRunNormalizesRecordedPlan(t *testing.T) {
	plan, err := os.ReadFile("testdata/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	x := exectest.New(t)
	x.On("terraform", []string{"init", "-input=false", "-no-color"}, exec.Output{})
	x.On("terraform", []string{"workspace", "select", "prod"}, exec.Output{})
	x.On("terraform", []string{"plan", "-out=.ai-flow-dryrun.tfplan", "-input=false", "-no-color", "-var-file=envs/prod.tfvars"}, exec.Output{})
	x.On("terraform", []string{"show", "-json", ".ai-flow-dryrun.tfplan"}, exec.Output{Stdout: string(plan)})

	cfg := &config.Config{Ops: config.Ops{Environments: map[string]config.Environment{
		"prod": {Tier: "prod", Terraform: &config.TerraformBinding{Workspace: "prod", VarFile: "envs/prod.tfvars"}},
	}}}
	a := terraform.New(x, cfg, "")
	t2 := ops.Target{Addon: "terraform", Repo: "acme.infra", Path: "infra/prod"}
	env := ops.Env{Name: "prod", Tier: "prod"}

	cs, err := a.DryRun(context.Background(), t2, env, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 5 {
		t.Fatalf("Actions = %d, want 5 (the data source read must be skipped)\n%+v", len(cs.Actions), cs.Actions)
	}

	byAddr := map[string]ops.Action{}
	for _, a := range cs.Actions {
		byAddr[a.Address] = a
	}

	if a := byAddr["aws_instance.web"]; a.Action != ops.ActionCreate || a.KindClass != ops.KindWorkload || len(a.FieldsChanged) != 0 {
		t.Errorf("aws_instance.web = %+v, unexpected", a)
	}
	if a := byAddr["aws_iam_role.web"]; a.Action != ops.ActionUpdate || a.KindClass != ops.KindIAM || len(a.FieldsChanged) != 1 || a.FieldsChanged[0] != "name" {
		t.Errorf("aws_iam_role.web = %+v, want update/iam/[name]", a)
	}
	if a := byAddr["aws_s3_bucket.old"]; a.Action != ops.ActionDelete || a.KindClass != ops.KindData {
		t.Errorf("aws_s3_bucket.old = %+v, want delete/data", a)
	}
	if a := byAddr["aws_db_instance.primary"]; a.Action != ops.ActionReplace || a.KindClass != ops.KindData || !a.Sensitive {
		t.Errorf("aws_db_instance.primary = %+v, want replace/data/sensitive", a)
	}
	if a := byAddr["aws_instance.noop"]; a.Action != ops.ActionNoop {
		t.Errorf("aws_instance.noop = %+v, want noop", a)
	}
	if _, ok := byAddr["data.aws_ami.ubuntu"]; ok {
		t.Error("data.aws_ami.ubuntu present, want the data source read skipped")
	}

	for _, action := range cs.Actions {
		if action.Protected {
			t.Errorf("%s.Protected = true, want always false from this addon (phase 23's own job)", action.Address)
		}
		if action.Target != "acme.infra:infra/prod" {
			t.Errorf("%s.Target = %q, want acme.infra:infra/prod", action.Address, action.Target)
		}
		if action.Env != "prod" || action.Tier != "prod" {
			t.Errorf("%s Env/Tier = %q/%q, want prod/prod", action.Address, action.Env, action.Tier)
		}
	}

	// Byte-stability of ordering: sorted by Address regardless of the
	// plan's own declaration order.
	for i := 1; i < len(cs.Actions); i++ {
		if cs.Actions[i-1].Address > cs.Actions[i].Address {
			t.Fatalf("Actions not sorted by Address: %q before %q", cs.Actions[i-1].Address, cs.Actions[i].Address)
		}
	}
}

func TestDryRunUnrecognizedActionsIsAnError(t *testing.T) {
	dir := t.TempDir()
	badPlan := `{"resource_changes":[{"address":"x.y","mode":"managed","type":"x","change":{"actions":["move"]}}]}`

	x := exectest.New(t)
	x.On("terraform", []string{"init", "-input=false", "-no-color"}, exec.Output{})
	x.On("terraform", []string{"plan", "-out=.ai-flow-dryrun.tfplan", "-input=false", "-no-color"}, exec.Output{})
	x.On("terraform", []string{"show", "-json", ".ai-flow-dryrun.tfplan"}, exec.Output{Stdout: badPlan})

	cfg := &config.Config{Ops: config.Ops{Environments: map[string]config.Environment{
		"dev": {Tier: "nonprod", Terraform: &config.TerraformBinding{Workspace: ""}},
	}}}
	a := terraform.New(x, cfg, "")
	_, err := a.DryRun(context.Background(), ops.Target{Path: "infra/dev"}, ops.Env{Name: "dev", Tier: "nonprod"}, &ops.Rendered{Dir: dir})
	if err == nil {
		t.Fatal("DryRun() err = nil, want the unrecognized actions combo rejected")
	}
}
