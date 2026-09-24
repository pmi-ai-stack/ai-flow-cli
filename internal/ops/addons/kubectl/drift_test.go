package kubectl

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

func boundConfig() *config.Config {
	return &config.Config{Ops: config.Ops{Environments: map[string]config.Environment{
		"prod": {Tier: "prod", Kube: &config.KubeBinding{Context: "prod-ctx"}},
	}}}
}

func TestDriftUnboundEnvIsLiveCredentialsMissingBeforeAnyCall(t *testing.T) {
	x := exectest.New(t)
	a := New(x, &config.Config{})

	_, err := a.Drift(context.Background(), ops.Target{}, ops.Env{}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("Drift error = %v, want ops.ErrLiveCredentialsMissing", err)
	}
	if len(x.Calls) != 0 {
		t.Fatalf("Drift made %d calls before checking the binding, want zero", len(x.Calls))
	}
}

func TestDriftNoDifferenceReturnsDriftNoneWithoutFurtherCalls(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n")

	x := exectest.New(t)
	x.On("kubectl", []string{"diff", "-f", ".", "--context", "prod-ctx"}, exec.Output{ExitCode: 0})

	a := New(x, boundConfig())
	report, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftNone || len(report.Resources) != 0 {
		t.Fatalf("Drift = %+v, want DriftNone with no resources", report)
	}
	if len(x.Calls) != 1 {
		t.Fatalf("Drift made %d calls, want exactly one (the presence-only diff)", len(x.Calls))
	}
}

func TestDriftPresentListsResourcesThatActuallyDiffer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n")
	writeFile(t, dir, "cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\n  namespace: default\ndata:\n  key: value\n")

	x := exectest.New(t)
	x.OnFailWithOutput("kubectl", []string{"diff", "-f", ".", "--context", "prod-ctx"},
		exec.Output{ExitCode: 1}, &envelope.Error{Code: "tool_failed", Message: "diff found changes"})
	getJSON := `{"kind":"List","items":[
		{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web","namespace":"default"},"spec":{"replicas":3}},
		{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cfg","namespace":"default"},"data":{"key":"value"}}
	]}`
	x.On("kubectl", []string{"get", "-f", ".", "-o", "json", "--ignore-not-found", "--context", "prod-ctx"}, exec.Output{Stdout: getJSON})

	a := New(x, boundConfig())
	report, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftPresent {
		t.Fatalf("Drift = %q, want present", report.Drift)
	}
	if len(report.Resources) != 1 || report.Resources[0] != "default/Deployment/web" {
		t.Fatalf("Resources = %v, want exactly [default/Deployment/web] (the ConfigMap is identical live, so not drifted)", report.Resources)
	}
}

func TestDriftMissingLiveResourceCountsAsDrifted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n")

	x := exectest.New(t)
	x.OnFailWithOutput("kubectl", []string{"diff", "-f", ".", "--context", "prod-ctx"},
		exec.Output{ExitCode: 1}, &envelope.Error{Code: "tool_failed", Message: "diff found changes"})
	x.On("kubectl", []string{"get", "-f", ".", "-o", "json", "--ignore-not-found", "--context", "prod-ctx"}, exec.Output{Stdout: ""})

	a := New(x, boundConfig())
	report, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Resources) != 1 || report.Resources[0] != "default/Deployment/web" {
		t.Fatalf("Resources = %v, want the resource with no live counterpart to count as drifted", report.Resources)
	}
}

func TestDriftGenuineFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n")

	x := exectest.New(t)
	x.OnFail("kubectl", []string{"diff", "-f", ".", "--context", "prod-ctx"}, &envelope.Error{Code: "internal", Message: "context deadline exceeded"})

	a := New(x, boundConfig())
	if _, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir}); err == nil {
		t.Fatal("Drift returned no error for a genuine tool failure")
	}
}
