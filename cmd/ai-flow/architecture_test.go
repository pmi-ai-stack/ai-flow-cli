package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArchitectureDocNamesEveryPackage keeps docs/architecture.md from
// going stale: every directory under internal/ that holds non-test Go
// code must be named there, as `internal/<path>/`. A new package fails
// this test until its one line is written.
func TestArchitectureDocNamesEveryPackage(t *testing.T) {
	root := filepath.Join("..", "..")
	doc, err := os.ReadFile(filepath.Join(root, "docs", "architecture.md"))
	if err != nil {
		t.Fatalf("read docs/architecture.md: %v", err)
	}
	text := string(doc)

	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || d.Name() == "testdata" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !hasNonTestGo(t, path) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel) + "/"
		if !strings.Contains(text, "`"+name+"`") {
			t.Errorf("docs/architecture.md does not name `%s`", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func hasNonTestGo(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			return true
		}
	}
	return false
}
