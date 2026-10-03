package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// usesLine matches a step's `uses:` value; pinnedRef is a full commit
// SHA, the only ref a third-party action cannot move under us.
var (
	usesLine        = regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s*(\S+)`)
	pinnedRef       = regexp.MustCompile(`@[0-9a-f]{40}$`)
	permissionsLine = regexp.MustCompile(`(?m)^permissions:`)
)

// TestWorkflowsPinActionsBySHA keeps phase 33's SEC-11 fix from
// regressing: every third-party action in this repository's workflows
// is pinned to a commit SHA (dependabot moves the pins), and every
// workflow declares its own least-privilege permissions instead of
// inheriting the repository default.
func TestWorkflowsPinActionsBySHA(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflows found (%v)", err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !permissionsLine.Match(data) {
			t.Errorf("%s: no top-level permissions: block", filepath.Base(f))
		}
		for _, m := range usesLine.FindAllStringSubmatch(string(data), -1) {
			ref := m[1]
			if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "docker://") {
				continue
			}
			if !pinnedRef.MatchString(ref) {
				t.Errorf("%s: %s is not pinned to a commit SHA", filepath.Base(f), ref)
			}
		}
	}
}
