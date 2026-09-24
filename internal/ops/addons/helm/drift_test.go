package helm

import (
	"context"
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestDriftUnboundEnvIsLiveCredentialsMissingBeforeAnyCall(t *testing.T) {
	x := exectest.New(t)
	a := New(x, &config.Config{})

	_, err := a.Drift(context.Background(), ops.Target{Path: "testdata/chart"}, ops.Env{}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("Drift error = %v, want ops.ErrLiveCredentialsMissing", err)
	}
	if len(x.Calls) != 0 {
		t.Fatalf("Drift made %d calls before checking the binding, want zero", len(x.Calls))
	}
}

func TestDriftNoDifferenceIsDriftNone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rendered.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n")

	x := exectest.New(t)
	x.On("helm", []string{"get", "manifest", "demo", "--kube-context", "prod-ctx"}, exec.Output{
		Stdout: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n",
	})

	a := New(x, boundConfig())
	report, err := a.Drift(context.Background(), ops.Target{Path: "testdata/chart"}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftNone || len(report.Resources) != 0 {
		t.Fatalf("Drift = %+v, want DriftNone", report)
	}
}

func TestDriftPresentListsDifferingAndMissingResources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rendered.yaml", ""+
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 3\n---\n"+
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\n  namespace: default\ndata:\n  key: value\n")

	x := exectest.New(t)
	x.On("helm", []string{"get", "manifest", "demo", "--kube-context", "prod-ctx"}, exec.Output{
		Stdout: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n",
	})

	a := New(x, boundConfig())
	report, err := a.Drift(context.Background(), ops.Target{Path: "testdata/chart"}, ops.Env{Name: "prod", Tier: "prod"}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftPresent || len(report.Resources) != 2 {
		t.Fatalf("Drift = %+v, want DriftPresent naming both resources (one changed, one missing live)", report)
	}
}
