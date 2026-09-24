package helm

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

const templateOutputWithSecret = "" +
	"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: default\nspec:\n  replicas: 2\n---\n" +
	"apiVersion: v1\nkind: Secret\nmetadata:\n  name: web-credentials\n  namespace: default\ndata:\n  password: czNjcjN0LXZhbHVl\nstringData:\n  token: plain-secret-token\n"

func TestRenderWritesTemplateOutputChartCopyAndMarksSecrets(t *testing.T) {
	x := exectest.New(t)
	x.On("helm", []string{"template", "demo", "testdata/chart", "-f", filepath.Join("testdata", "chart", "values.yaml")}, exec.Output{Stdout: templateOutputWithSecret})

	a := New(x, &config.Config{})
	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Path: "testdata/chart"}, ops.RefBranch, out)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(out, "rendered.yaml"))
	if err != nil || string(data) != templateOutputWithSecret {
		t.Fatalf("rendered.yaml = %q, err %v, want exactly helm template's own stdout", data, err)
	}

	if _, err := os.Stat(filepath.Join(out, "chart", "Chart.yaml")); err != nil {
		t.Errorf("Render did not copy Chart.yaml into out/chart/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "chart", "templates", "deployment.yaml")); err != nil {
		t.Errorf("Render did not recursively copy templates/deployment.yaml into out/chart/: %v", err)
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

func TestRenderOmitsValuesFlagWhenNoValuesFile(t *testing.T) {
	chartDir := t.TempDir()
	writeFile(t, chartDir, "Chart.yaml", "apiVersion: v2\nname: novalues\nversion: 0.1.0\n")

	x := exectest.New(t)
	x.On("helm", []string{"template", "novalues", chartDir}, exec.Output{Stdout: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\n"})

	a := New(x, &config.Config{})
	if _, err := a.Render(context.Background(), ops.Target{Path: chartDir}, ops.RefBranch, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
