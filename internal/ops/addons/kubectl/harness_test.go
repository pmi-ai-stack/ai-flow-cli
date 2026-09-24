package kubectl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestHarnessesFindsKuttlAndChainsawSuitesRecursivelyNoSubprocess(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "manifests", "tests", "kuttl-case"))
	mustMkdirAll(t, filepath.Join(repo, "manifests", "tests", "chainsaw-case"))
	writeFile(t, filepath.Join(repo, "manifests", "tests", "kuttl-case"), "kuttl-test.yaml", "testDirs:\n  - .\n")
	writeFile(t, filepath.Join(repo, "manifests", "tests", "chainsaw-case"), "chainsaw-test.yaml", "apiVersion: chainsaw.kyverno.io/v1alpha1\nkind: Test\n")
	writeFile(t, filepath.Join(repo, "manifests"), "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n")

	x := exectest.New(t) // asserts zero calls if Harnesses ever tries to run kuttl/chainsaw
	a := New(x, nil)
	harnesses, err := a.Harnesses(detect.Repo{LocalPath: repo}, ops.Target{Path: "manifests"})
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 2 {
		t.Fatalf("Harnesses = %v, want two (one kuttl, one chainsaw)", harnesses)
	}
	for _, h := range harnesses {
		if h.Kind != ops.HarnessE2E {
			t.Errorf("harness %+v has Kind %q, want e2e (kubectl has no unit harness)", h, h.Kind)
		}
	}
	if len(x.Calls) != 0 {
		t.Fatalf("Harnesses made %d subprocess calls, want zero — detection only", len(x.Calls))
	}
}

func TestHarnessesEmptyWhenNoSuiteFiles(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "manifests"))
	writeFile(t, filepath.Join(repo, "manifests"), "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n")

	a := New(nil, nil)
	harnesses, err := a.Harnesses(detect.Repo{LocalPath: repo}, ops.Target{Path: "manifests"})
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 0 {
		t.Fatalf("Harnesses = %v, want none", harnesses)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}
