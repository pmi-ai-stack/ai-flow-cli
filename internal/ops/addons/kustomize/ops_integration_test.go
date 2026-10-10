//go:build ops

package kustomize

import (
	"context"
	"os"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// TestRealKustomizeDetectRenderValidateHarnesses drives Detect, Render
// and Harnesses against real "kustomize"/"kubeconform" binaries when
// present — confirmed present ("kustomize") / absent ("kubeconform")
// in this development environment, see .ai/phase-26/plan.md §0.1.
// Drift/DryRun/Rollback are not exercised here: no live cluster is
// reachable in this environment, the same carried-forward gap phases
// 21/22/24 each already disclosed for their own live-tool paths.
func TestRealKustomizeDetectRenderValidateHarnesses(t *testing.T) {
	exectest.RequireTool(t, "kustomize")

	a := New(exec.NewRunner(), &config.Config{Ops: config.Ops{ToolTimeoutMinutes: 1}})

	targets, err := a.Detect(detect.Repo{Name: "repo", LocalPath: "testdata"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var overlay *ops.Target
	for i, tg := range targets {
		if tg.Path == "overlay" {
			overlay = &targets[i]
		}
	}
	if overlay == nil {
		t.Fatalf("Detect = %v, want an \"overlay\" target", targets)
	}

	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Path: "testdata/overlay"}, ops.RefBranch, out)
	if err != nil {
		t.Fatalf("real kustomize build failed: %v", err)
	}
	data, err := os.ReadFile(out + "/rendered.yaml")
	if err != nil || len(data) == 0 {
		t.Fatalf("rendered.yaml missing or empty: %v", err)
	}

	if !exectest.HasTool(t, "kubeconform") {
		// Validate is kubeconform; the rest of the test ran without it.
	} else if _, err := a.Validate(context.Background(), rendered); err != nil {
		t.Errorf("real kubeconform failed: %v", err)
	}

	if _, err := a.Harnesses(detect.Repo{LocalPath: "testdata"}, *overlay); err != nil {
		t.Errorf("Harnesses failed: %v", err)
	}
}
