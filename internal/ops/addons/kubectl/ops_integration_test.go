//go:build ops

package kubectl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/kubectl"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// TestRealKubectlDetectRenderValidateHarnesses is the spec's own
// "-tags ops runs real kubectl against a kind cluster when present"
// Done when clause — skipped, not failed, when either kubectl or
// kubeconform is absent (the same posture terraform's own
// ops_integration_test.go already established for tflint). No live
// cluster is required for Detect/Render/Validate/Harnesses (Render is
// a plain copy; Validate is purely static); Drift/DryRun's own
// real-live-state path additionally needs a reachable kind cluster and
// is left to a human running this tag locally against one, mirroring
// terraform's own Drift/DryRun exclusion from this same test for the
// identical reason (no safe, credential-free fixture to run against in
// CI).
func TestRealKubectlDetectRenderValidateHarnesses(t *testing.T) {
	exectest.RequireTool(t, "kubectl")
	exectest.RequireTool(t, "kubeconform")

	repoDir := t.TempDir()
	src, err := os.ReadFile("testdata/manifests/deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	manifestsDir := filepath.Join(repoDir, "infra", "manifests")
	if err := os.MkdirAll(manifestsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestsDir, "deployment.yaml"), src, 0o600); err != nil {
		t.Fatal(err)
	}

	a := kubectl.New(exec.NewRunner(), &config.Config{})

	targets, err := a.Detect(detect.Repo{Name: "acme.k8s", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly one manifests directory", targets)
	}

	out := t.TempDir()
	rendered, err := a.Render(context.Background(), ops.Target{Addon: "kubectl", Path: manifestsDir}, ops.RefBranch, out)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	findings, err := a.Validate(context.Background(), rendered)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("Validate on the valid fixture = %v, want no findings", findings)
	}

	// An invalid manifest must come back as a finding: before phase 58
	// Validate passed no path (kubeconform read stdin and reported
	// nothing) and matched status names kubeconform never prints.
	invalid := t.TempDir()
	bad := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: broken\nspec:\n  replicas: \"three\"\n"
	if err := os.WriteFile(filepath.Join(invalid, "bad.yaml"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err = a.Validate(context.Background(), &ops.Rendered{Dir: invalid})
	if err != nil {
		t.Fatalf("Validate (invalid manifest): %v", err)
	}
	if len(findings) != 1 || findings[0].Severity != "error" || findings[0].File != "bad.yaml" {
		t.Errorf("Validate (invalid manifest) = %+v, want one error finding for bad.yaml", findings)
	}

	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.k8s", LocalPath: repoDir}, targets[0])
	if err != nil {
		t.Fatalf("Harnesses: %v", err)
	}
	if len(harnesses) != 0 {
		t.Errorf("harnesses = %v, want none (the fixture manifests ship no kuttl-test.yaml/chainsaw-test.yaml)", harnesses)
	}
}
