package kustomize

import (
	"context"
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestDryRunUnboundEnvIsLiveCredentialsMissingBeforeAnyCall(t *testing.T) {
	x := exectest.New(t)
	a := New(x, &config.Config{})

	_, err := a.DryRun(context.Background(), ops.Target{}, ops.Env{}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("DryRun error = %v, want ops.ErrLiveCredentialsMissing", err)
	}
	if len(x.Calls) != 0 {
		t.Fatalf("DryRun made %d calls before checking the binding, want zero", len(x.Calls))
	}
}

func TestDryRunNoDifferenceProducesAllNoopChangeset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rendered.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n")

	x := exectest.New(t)
	x.On("kubectl", []string{"diff", "--server-side", "-f", ".", "--context", "prod-ctx"}, exec.Output{ExitCode: 0})

	a := New(x, boundConfig())
	cs, err := a.DryRun(context.Background(), ops.Target{Repo: "r", Path: "overlays/prod"}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 1 || cs.Actions[0].Action != ops.ActionNoop {
		t.Fatalf("Changeset = %+v, want one noop action", cs.Actions)
	}
	if cs.Actions[0].Addon != "kustomize" || cs.Actions[0].Target != "r:overlays/prod" {
		t.Errorf("action = %+v, want Addon kustomize, Target r:overlays/prod", cs.Actions[0])
	}
}

func TestDryRunBuildsCreateUpdateAndNoopActions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rendered.yaml", ""+
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 3\n---\n"+
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\n  namespace: default\ndata:\n  key: value\n")

	x := exectest.New(t)
	x.OnFailWithOutput("kubectl", []string{"diff", "--server-side", "-f", ".", "--context", "prod-ctx"},
		exec.Output{ExitCode: 1}, &envelope.Error{Code: "tool_failed", Message: "diff found changes"})

	getJSON := `{"kind":"List","items":[
		{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default"},"spec":{"replicas":2}}
	]}`
	x.On("kubectl", []string{"get", "-f", ".", "-o", "json", "--ignore-not-found", "--context", "prod-ctx"}, exec.Output{Stdout: getJSON})

	applyJSON := `{"kind":"List","items":[
		{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default"},"spec":{"replicas":3}},
		{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cfg","namespace":"default"},"data":{"key":"value"}}
	]}`
	x.On("kubectl", []string{"apply", "-f", ".", "--dry-run=server", "-o", "json", "--context", "prod-ctx"}, exec.Output{Stdout: applyJSON})

	a := New(x, boundConfig())
	cs, err := a.DryRun(context.Background(), ops.Target{Repo: "r", Path: "overlays/prod"}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 2 {
		t.Fatalf("Changeset.Actions = %+v, want 2", cs.Actions)
	}

	byAddress := map[string]ops.Action{}
	for _, act := range cs.Actions {
		byAddress[act.Address] = act
	}
	if a := byAddress["default/ConfigMap/cfg"]; a.Action != ops.ActionCreate {
		t.Errorf("ConfigMap action = %+v, want create", a)
	}
	if a := byAddress["default/Deployment/web"]; a.Action != ops.ActionUpdate || len(a.FieldsChanged) != 1 || a.FieldsChanged[0] != "spec" {
		t.Errorf("Deployment action = %+v, want update with fields_changed [spec]", a)
	}
	for _, act := range cs.Actions {
		if act.Sensitive || act.Protected {
			t.Errorf("action %+v has Sensitive/Protected true, want both false from this addon", act)
		}
	}
}
