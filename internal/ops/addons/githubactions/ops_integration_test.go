//go:build ops

package githubactions_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// TestRealActionlintDetectRenderValidate is the spec's own "-tags ops
// runs the real tools when present" Done when clause — skipped, not
// failed, when either binary is absent, mirroring every prior addon
// phase's own integration test. DryRun's own real "act -n" call is
// deliberately NOT exercised here (unlike Detect/Render/Validate/
// Harnesses): act's own first-run-per-machine interactive image
// selection (confirmed for real while writing this addon, see
// .ai/phase-27/plan.md §0.1) would hang an unconfigured CI runner —
// the identical "left to a human running this tag locally against a
// real, configured environment" posture terraform's own integration
// test already carries for its own live-backend path.
func TestRealActionlintDetectRenderValidate(t *testing.T) {
	exectest.RequireTool(t, "actionlint")

	repoDir := t.TempDir()
	src, err := os.ReadFile("testdata/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	wfPath := filepath.Join(repoDir, ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(wfPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wfPath, src, 0o600); err != nil {
		t.Fatal(err)
	}

	a := githubactions.New(exec.NewRunner(), &config.Config{})

	targets, err := a.Detect(detect.Repo{Name: "acme.svc", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly one workflow", targets)
	}

	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Addon: "github-actions", Path: wfPath}, ops.RefBranch, out)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if _, err := a.Validate(context.Background(), rendered); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.svc", LocalPath: repoDir}, targets[0])
	if err != nil {
		t.Fatalf("Harnesses: %v", err)
	}
	if len(harnesses) != 2 {
		t.Errorf("harnesses = %v, want 2 (unit+e2e for the fixture's one job)", harnesses)
	}
}
