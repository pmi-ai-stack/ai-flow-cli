package helm

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestHarnessesDetectsAllThreeSignalKindsNoSubprocess(t *testing.T) {
	repo := t.TempDir()
	chartDir := filepath.Join(repo, "chart")
	mustMkdirAll(t, filepath.Join(chartDir, "tests"))
	mustMkdirAll(t, filepath.Join(chartDir, "ci"))
	writeFile(t, chartDir, "Chart.yaml", "apiVersion: v2\nname: demo\nversion: 0.1.0\n")
	writeFile(t, filepath.Join(chartDir, "tests"), "deployment_test.yaml", "suite: deployment\n")
	writeFile(t, filepath.Join(chartDir, "ci"), "default-values.yaml", "replicas: 1\n")

	x := exectest.New(t)
	a := New(x, nil)
	harnesses, err := a.Harnesses(detect.Repo{LocalPath: repo}, ops.Target{Path: "chart"})
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 3 {
		t.Fatalf("Harnesses = %v, want three (Chart.yaml for ct lint, one unittest file, one ci case)", harnesses)
	}

	var unit, e2e int
	for _, h := range harnesses {
		switch h.Kind {
		case ops.HarnessUnit:
			unit++
		case ops.HarnessE2E:
			e2e++
		}
	}
	if unit != 2 || e2e != 1 {
		t.Fatalf("Harnesses kinds = unit:%d e2e:%d, want unit:2 (Chart.yaml + tests file) e2e:1 (ci case)", unit, e2e)
	}
	if len(x.Calls) != 0 {
		t.Fatalf("Harnesses made %d subprocess calls, want zero — detection only", len(x.Calls))
	}
}

func TestHarnessesChartAloneIsStillOneCtLintSignal(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "chart"))
	writeFile(t, filepath.Join(repo, "chart"), "Chart.yaml", "apiVersion: v2\nname: demo\nversion: 0.1.0\n")

	a := New(nil, nil)
	harnesses, err := a.Harnesses(detect.Repo{LocalPath: repo}, ops.Target{Path: "chart"})
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 1 || harnesses[0].Kind != ops.HarnessUnit {
		t.Fatalf("Harnesses = %v, want exactly one HarnessUnit (the chart's own Chart.yaml, ct lint's signal)", harnesses)
	}
}
