package terraform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
)

func TestDetectFindsRootModuleOnly(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(repoDir, "infra", "prod", "main.tf"))
	mustCopyFile(t, "testdata/notmodule/vars.tf", filepath.Join(repoDir, "infra", "vars", "vars.tf"))

	a := terraform.New(nil, nil, "")
	targets, err := a.Detect(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly one root module", targets)
	}
	if targets[0].Path != "infra/prod" {
		t.Errorf("Path = %q, want infra/prod", targets[0].Path)
	}
	if targets[0].Addon != "terraform" {
		t.Errorf("Addon = %q, want terraform", targets[0].Addon)
	}
}

func TestDetectSkipsDotTerraformCache(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(repoDir, "infra", "prod", "main.tf"))
	// A stray .terraform/ left behind by a prior real run must never be
	// mistaken for a second root module, even though it can itself
	// contain .tf-shaped provider cache files in principle.
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(repoDir, "infra", "prod", ".terraform", "modules", "x", "main.tf"))

	a := terraform.New(nil, nil, "")
	targets, err := a.Detect(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly one (the .terraform/ cache must be skipped)", targets)
	}
}

func TestDetectFiltersByChanged(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/module/main.tf", filepath.Join(repoDir, "infra", "prod", "main.tf"))

	a := terraform.New(nil, nil, "")
	changed := []string{"infra/prod/main.tf", "README.md"}
	targets, err := a.Detect(detect.Repo{Name: "acme.infra", LocalPath: repoDir}, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || len(targets[0].Files) != 1 || targets[0].Files[0] != "infra/prod/main.tf" {
		t.Fatalf("targets = %v, want one target claiming exactly infra/prod/main.tf", targets)
	}
}

func mustCopyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
