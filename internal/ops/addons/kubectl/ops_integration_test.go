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
	if _, ok := exec.Which("kubectl"); !ok {
		t.Skip("kubectl not on PATH")
	}
	if _, ok := exec.Which("kubeconform"); !ok {
		t.Skip("kubeconform not on PATH")
	}

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

	if _, err := a.Validate(context.Background(), rendered); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.k8s", LocalPath: repoDir}, targets[0])
	if err != nil {
		t.Fatalf("Harnesses: %v", err)
	}
	if len(harnesses) != 0 {
		t.Errorf("harnesses = %v, want none (the fixture manifests ship no kuttl-test.yaml/chainsaw-test.yaml)", harnesses)
	}
}
