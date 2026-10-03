package render_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

func TestTreeHashIdenticalContentSameHashRegardlessOfWalkOrder(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	files := map[string]string{
		"a.txt":          "one",
		"sub/b.txt":      "two",
		"sub/deep/c.txt": "three",
	}
	for name, content := range files {
		for _, root := range []string{a, b} {
			path := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	ha, err := render.TreeHash(a, nil)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := render.TreeHash(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("TreeHash(a) = %q, TreeHash(b) = %q, want equal for identical content", ha, hb)
	}
}

func TestTreeHashChangesWhenContentChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	h1, err := render.TreeHash(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	h2, err := render.TreeHash(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Errorf("TreeHash did not change after content changed: both %q", h1)
	}
}

// TestTreeHashSkipDirExcludesSubtreeButNeverRoot pins the skip
// predicate's contract: nothing beneath a skipped directory moves the
// hash, content outside it still does, and root itself is never
// skipped even when its own name matches (a .github/workflows target).
func TestTreeHashSkipDirExcludesSubtreeButNeverRoot(t *testing.T) {
	skipDot := func(name string) bool { return name != "" && name[0] == '.' }
	root := filepath.Join(t.TempDir(), ".root-is-a-dot-dir")
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("main.tf", "resource {}")
	clean, err := render.TreeHash(root, skipDot)
	if err != nil {
		t.Fatal(err)
	}

	write(".terraform/providers/huge-binary", "provider bytes")
	write("modules/.cache/x", "cache")
	withCache, err := render.TreeHash(root, skipDot)
	if err != nil {
		t.Fatal(err)
	}
	if withCache != clean {
		t.Errorf("hash moved after adding only skipped directories: %q → %q", clean, withCache)
	}
	if unskipped, err := render.TreeHash(root, nil); err != nil || unskipped == clean {
		t.Errorf("TreeHash(root, nil) = %q, %v; want a different hash that includes the dot-directories", unskipped, err)
	}

	write("main.tf", "resource { changed = true }")
	edited, err := render.TreeHash(root, skipDot)
	if err != nil {
		t.Fatal(err)
	}
	if edited == clean {
		t.Error("hash did not move after editing a non-skipped file")
	}
}
