//go:build ops

package terraform_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// TestRealTerraformDetectRenderValidate is the spec's own "-tags ops
// runs real terraform when it is on PATH" Done when clause — skipped,
// not failed, when either binary is absent (the same posture
// internal/verify/unit's -tags integration suite already established
// for a missing git). No live backend is configured for the fixture
// module (its own "local" backend needs no credentials at all), so
// only Detect/Render/Validate/Harnesses are exercised here — Drift/
// DryRun's own real-live-state path has no safe, credential-free
// fixture to run against in CI and is left to a human running this
// tag locally against a real, credentialed environment.
func TestRealTerraformDetectRenderValidate(t *testing.T) {
	if _, ok := exec.Which("terraform"); !ok {
		t.Skip("terraform not on PATH")
	}
	if _, ok := exec.Which("tflint"); !ok {
		t.Skip("tflint not on PATH")
	}

	repoDir := t.TempDir()
	src, err := os.ReadFile("testdata/module/main.tf")
	if err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(repoDir, "infra", "prod")
	if err := os.MkdirAll(moduleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), src, 0o600); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	a := terraform.New(exec.NewRunner(), &config.Config{}, home)

	targets, err := a.Detect(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly one root module", targets)
	}

	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Addon: "terraform", Path: moduleDir}, ops.RefBranch, out)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if _, err := a.Validate(context.Background(), rendered); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, targets[0])
	if err != nil {
		t.Fatalf("Harnesses: %v", err)
	}
	if len(harnesses) != 0 {
		t.Errorf("harnesses = %v, want none (the fixture module ships no .tftest.hcl)", harnesses)
	}
}
