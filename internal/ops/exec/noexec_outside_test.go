package exec

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoOsExecOutsideExecPackage asserts os/exec is imported nowhere in
// this module except this package — the CLI's one rule for how
// subprocesses are run (Go coding rules #6; the CI grep this test
// replaces with something that runs offline in `go test`).
func TestNoOsExecOutsideExecPackage(t *testing.T) {
	root := moduleRoot(t)
	thisPkgRel := filepath.Join("internal", "ops", "exec")

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == thisPkgRel || strings.HasPrefix(rel, thisPkgRel+string(filepath.Separator)) {
			return nil // this package is the one place os/exec is allowed
		}

		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(b, []byte(`"os/exec"`)) {
			t.Errorf("%s imports os/exec; every subprocess must go through internal/ops/exec", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// moduleRoot walks up from the test's working directory (the package
// directory under `go test`) to the nearest go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the exec package directory")
		}
		dir = parent
	}
}
