package kustomize

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

const buildOutputWithSecret = "" +
	"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n---\n" +
	"apiVersion: v1\nkind: Secret\nmetadata:\n  name: web-credentials\n  namespace: default\ndata:\n  password: czNjcjN0LXZhbHVl\nstringData:\n  token: plain-secret-token\n"

func TestRenderWritesBuildOutputAndMarksSecretValues(t *testing.T) {
	x := exectest.New(t)
	x.On("kustomize", []string{"build", "overlays/prod"}, exec.Output{Stdout: buildOutputWithSecret})

	a := New(x, &config.Config{})
	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Path: "overlays/prod"}, ops.RefBranch, out)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(out, "rendered.yaml"))
	if err != nil {
		t.Fatalf("Render did not write rendered.yaml: %v", err)
	}
	if string(data) != buildOutputWithSecret {
		t.Errorf("rendered.yaml content = %q, want exactly kustomize build's own stdout", data)
	}

	var values []string
	for _, m := range rendered.Marked {
		values = append(values, m.Value)
	}
	sort.Strings(values)
	want := []string{"czNjcjN0LXZhbHVl", "plain-secret-token"}
	if len(values) != len(want) || values[0] != want[0] || values[1] != want[1] {
		t.Fatalf("Rendered.Marked values = %v, want %v", values, want)
	}
}

func TestRenderMarksNothingWhenNoSecretPresent(t *testing.T) {
	x := exectest.New(t)
	x.On("kustomize", []string{"build", "base"}, exec.Output{Stdout: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\ndata:\n  key: value\n"})

	a := New(x, &config.Config{})
	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Path: "base"}, ops.RefBranch, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.Marked) != 0 {
		t.Errorf("Rendered.Marked = %v, want none — a ConfigMap is never marked, only a Secret", rendered.Marked)
	}
}

func TestRenderPropagatesBuildFailure(t *testing.T) {
	x := exectest.New(t)
	x.OnFail("kustomize", []string{"build", "broken"}, os.ErrInvalid)

	a := New(x, &config.Config{})
	if _, err := a.Render(context.Background(), ops.Target{Path: "broken"}, ops.RefBranch, t.TempDir()); err == nil {
		t.Fatal("Render returned no error for a failed kustomize build")
	}
}
