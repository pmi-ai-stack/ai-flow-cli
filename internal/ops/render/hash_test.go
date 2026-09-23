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

	ha, err := render.TreeHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := render.TreeHash(b)
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
	h1, err := render.TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	h2, err := render.TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Errorf("TreeHash did not change after content changed: both %q", h1)
	}
}
