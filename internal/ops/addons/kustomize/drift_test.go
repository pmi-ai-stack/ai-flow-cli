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

func TestDriftNoDifferenceIsDriftNoneWithoutFurtherCalls(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rendered.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\n")

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

func TestDriftPresentListsDifferingResources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rendered.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n")

	x := exectest.New(t)
	x.OnFailWithOutput("kubectl", []string{"diff", "-f", ".", "--context", "prod-ctx"},
		exec.Output{ExitCode: 1}, &envelope.Error{Code: "tool_failed", Message: "diff found changes"})
	x.On("kubectl", []string{"get", "-f", ".", "-o", "json", "--ignore-not-found", "--context", "prod-ctx"},
		exec.Output{Stdout: `{"kind":"Deployment","metadata":{"name":"web","namespace":"default"},"spec":{"replicas":1}}`})

	a := New(x, boundConfig())
	report, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftPresent || len(report.Resources) != 1 || report.Resources[0] != "default/Deployment/web" {
		t.Fatalf("Drift = %+v, want DriftPresent naming default/Deployment/web", report)
	}
}
