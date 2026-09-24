package githubactions_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
)

func TestDetectFindsWorkflowFilesOnly(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/ci.yml", filepath.Join(repoDir, ".github", "workflows", "ci.yml"))
	mustCopyFile(t, "testdata/ci-two-jobs.yml", filepath.Join(repoDir, ".github", "workflows", "release.yaml"))
	mustCopyFile(t, "testdata/ci.yml", filepath.Join(repoDir, "docs", "notaworkflow.yml"))

	a := githubactions.New(nil, devConfig())
	targets, err := a.Detect(detect.Repo{Name: "acme.svc", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %v, want exactly two workflow files", targets)
	}
	if targets[0].Path != ".github/workflows/ci.yml" || targets[0].Addon != "github-actions" {
		t.Errorf("targets[0] = %+v, want .github/workflows/ci.yml", targets[0])
	}
	if targets[1].Path != ".github/workflows/release.yaml" {
		t.Errorf("targets[1] = %+v, want .github/workflows/release.yaml", targets[1])
	}
}

func TestDetectNoWorkflowsDirIsEmptyNotError(t *testing.T) {
	a := githubactions.New(nil, devConfig())
	targets, err := a.Detect(detect.Repo{Name: "acme.svc", LocalPath: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Errorf("targets = %v, want none", targets)
	}
}

func TestDetectFiltersByChanged(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/ci.yml", filepath.Join(repoDir, ".github", "workflows", "ci.yml"))
	mustCopyFile(t, "testdata/ci.yml", filepath.Join(repoDir, ".github", "workflows", "release.yml"))

	a := githubactions.New(nil, devConfig())
	changed := []string{".github/workflows/ci.yml", "README.md"}
	targets, err := a.Detect(detect.Repo{Name: "acme.svc", LocalPath: repoDir}, changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %v, want both files still detected at scope repo/diff", targets)
	}
	if len(targets[0].Files) != 1 || targets[0].Files[0] != ".github/workflows/ci.yml" {
		t.Errorf("targets[0].Files = %v, want exactly the changed ci.yml", targets[0].Files)
	}
	if len(targets[1].Files) != 0 {
		t.Errorf("targets[1].Files = %v, want none (release.yml did not change)", targets[1].Files)
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
