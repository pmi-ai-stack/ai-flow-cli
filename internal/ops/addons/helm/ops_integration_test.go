//go:build ops

package helm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// TestRealHelmDetectRenderValidateHarnesses drives Detect, Render,
// Validate and Harnesses against the real "helm" binary (confirmed
// present, v3.16.2 — see .ai/phase-26/plan.md §0.1) and "kubeconform"
// (confirmed absent, skips cleanly). Drift/DryRun/Rollback are not
// exercised here: no live cluster is reachable in this environment,
// the same carried-forward gap phases 21/22/24 each already disclosed.
func TestRealHelmDetectRenderValidateHarnesses(t *testing.T) {
	exectest.RequireTool(t, "helm")

	a := New(exec.NewRunner(), &config.Config{Ops: config.Ops{ToolTimeoutMinutes: 1}})

	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Path != "chart" {
		t.Fatalf("Detect = %v, want exactly one target at \"chart\"", targets)
	}

	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Path: "testdata/chart"}, ops.RefBranch, out)
	if err != nil {
		t.Fatalf("real helm template failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "rendered.yaml"))
	if err != nil || len(data) == 0 {
		t.Fatalf("rendered.yaml missing or empty: %v", err)
	}
	if len(rendered.Marked) != 2 {
		t.Fatalf("Rendered.Marked = %v, want two (the fixture chart's own Secret data/stringData)", rendered.Marked)
	}

	findings, err := a.runHelmLint(context.Background(), out)
	if err != nil {
		t.Fatalf("real helm lint failed: %v", err)
	}
	for _, f := range findings {
		t.Logf("helm lint finding: %+v", f)
	}

	if !exectest.HasTool(t, "kubeconform") {
		// the helm lint half above already ran.
	} else if _, err := a.runKubeconform(context.Background(), out); err != nil {
		t.Errorf("real kubeconform failed: %v", err)
	}

	if _, err := a.Harnesses(detect.Repo{LocalPath: "testdata"}, targets[0]); err != nil {
		t.Errorf("Harnesses failed: %v", err)
	}
}
