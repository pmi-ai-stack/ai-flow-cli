package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLint_SoftBodyWarning proves the soft body-length threshold (150
// lines for a stage skill) surfaces as a Result.Warning, not a Finding
// — distinct from RuleBodyTooLong's own hard, 500-line hard-failure
// fixture (testdata/skills/invalid-body-too-long).
func TestLint_SoftBodyWarning(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "run-mutation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	var body strings.Builder
	body.WriteString("---\nname: run-mutation\ndescription: A. Loaded by the develop orchestrator at the mutation stage; not for direct use.\nlicense: MIT\ncompatibility: Requires the ai-flow CLI on PATH and a git checkout of the in-scope repositories.\n---\n\n")
	for i := 0; i < 200; i++ {
		body.WriteString("Filler line.\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Lint(root)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	for _, f := range res.Findings {
		if f.Rule == RuleBodyTooLong {
			t.Errorf("200-line body produced a hard Finding, want only a soft Warning: %+v", f)
		}
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", res.Warnings)
	}
}
