package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// sandboxPATH points PATH at an empty temp directory for the duration
// of the test, then restores it on cleanup — proves Which's not-found
// branch offline, with no dependence on what happens to be installed
// on the host running the test.
func sandboxPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir)
}

func TestWhichFound(t *testing.T) {
	dir := t.TempDir()
	name := "ai-flow-which-fixture"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fixture := filepath.Join(dir, name)
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\n"), 0o755); err != nil { // a throwaway PATH fixture, not a real artifact
		t.Fatalf("WriteFile: %v", err)
	}
	sandboxPATH(t, dir)

	path, found := Which(name)
	if !found {
		t.Fatalf("Which(%q): found = false, want true", name)
	}
	if path == "" {
		t.Error("Which: path is empty on a found result")
	}
}

func TestWhichNotFound(t *testing.T) {
	sandboxPATH(t, t.TempDir())

	_, found := Which("ai-flow-which-does-not-exist")
	if found {
		t.Error("Which: found = true against an empty PATH, want false")
	}
}
