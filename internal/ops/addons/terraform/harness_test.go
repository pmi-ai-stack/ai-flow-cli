package terraform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
)

func TestHarnessesFindsTftestFilesRecursively(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(repoDir, "infra", "prod", "main.tf"))
	mustWriteFile(t, filepath.Join(repoDir, "infra", "prod", "main.tftest.hcl"), "run \"smoke\" {}\n")
	mustWriteFile(t, filepath.Join(repoDir, "infra", "prod", "tests", "extra.tftest.hcl"), "run \"extra\" {}\n")
	mustWriteFile(t, filepath.Join(repoDir, "infra", "prod", "notes.txt"), "not a harness\n")

	a := terraform.New(nil, nil, "")
	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, ops.Target{Path: "infra/prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 2 {
		t.Fatalf("harnesses = %v, want 2", harnesses)
	}
	if harnesses[0].Name != "infra/prod/main.tftest.hcl" || harnesses[0].Kind != ops.HarnessUnit {
		t.Errorf("harnesses[0] = %+v, unexpected", harnesses[0])
	}
	if harnesses[1].Name != "infra/prod/tests/extra.tftest.hcl" {
		t.Errorf("harnesses[1] = %+v, unexpected", harnesses[1])
	}
}

func TestHarnessesEmptyIsNonNil(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(repoDir, "infra", "prod", "main.tf"))

	a := terraform.New(nil, nil, "")
	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, ops.Target{Path: "infra/prod"})
	if err != nil {
		t.Fatal(err)
	}
	if harnesses == nil || len(harnesses) != 0 {
		t.Errorf("harnesses = %#v, want an empty, non-nil slice", harnesses)
	}
}

// TestHarnessesDetectsTerratestFiles is the phase-24 addition: a
// "*_test.go" file importing Terratest is a Kind: HarnessE2E harness,
// ordered after every Kind: HarnessUnit .tftest.hcl harness; a plain
// Go test file with no Terratest import is not a harness at all.
func TestHarnessesDetectsTerratestFiles(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(repoDir, "infra", "prod", "main.tf"))
	mustWriteFile(t, filepath.Join(repoDir, "infra", "prod", "main.tftest.hcl"), "run \"smoke\" {}\n")
	mustWriteFile(t, filepath.Join(repoDir, "infra", "prod", "sandbox_test.go"), "package prod_test\n\nimport (\n\t\"testing\"\n\n\t\"github.com/gruntwork-io/terratest/modules/terraform\"\n)\n\nfunc TestSandbox(t *testing.T) { _ = terraform.Options{} }\n")
	mustWriteFile(t, filepath.Join(repoDir, "infra", "prod", "helpers_test.go"), "package prod_test\n\nimport \"testing\"\n\nfunc TestHelper(t *testing.T) {}\n")

	a := terraform.New(nil, nil, "")
	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, ops.Target{Path: "infra/prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 2 {
		t.Fatalf("harnesses = %+v, want one unit + one e2e harness (helpers_test.go must not count)", harnesses)
	}
	if harnesses[0].Name != "infra/prod/main.tftest.hcl" || harnesses[0].Kind != ops.HarnessUnit {
		t.Errorf("harnesses[0] = %+v, want the .tftest.hcl unit harness first", harnesses[0])
	}
	if harnesses[1].Name != "infra/prod/sandbox_test.go" || harnesses[1].Kind != ops.HarnessE2E {
		t.Errorf("harnesses[1] = %+v, want the Terratest e2e harness", harnesses[1])
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
