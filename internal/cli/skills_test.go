package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsLint_ValidFixture_Passes(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "skills", "valid")
	stdout, _, exitCode := runRoot(t, "skills", "lint", dir)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Skills   []string `json:"skills"`
			Findings []any    `json:"findings"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if !env.OK {
		t.Fatalf("ok = false, want true: %s", stdout.String())
	}
	if len(env.Result.Findings) != 0 {
		t.Errorf("Findings = %v, want empty", env.Result.Findings)
	}
	if len(env.Result.Skills) != 3 {
		t.Errorf("Skills = %v, want 3 entries", env.Result.Skills)
	}
}

func TestSkillsLint_InvalidFixture_Exit6WithRuleInDetails(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "skills", "invalid-name-matches-directory")
	stdout, _, exitCode := runRoot(t, "skills", "lint", dir)
	if exitCode != 6 {
		t.Fatalf("exitCode = %d, want 6\n%s", exitCode, stdout.String())
	}

	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string   `json:"code"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.OK {
		t.Fatalf("ok = true, want false")
	}
	if env.Error.Code != "invalid_result" {
		t.Errorf("error.code = %q, want %q", env.Error.Code, "invalid_result")
	}
	if len(env.Error.Details) != 1 || !strings.HasPrefix(env.Error.Details[0], "name-matches-directory:") {
		t.Errorf("error.details = %v, want exactly one entry prefixed \"name-matches-directory:\"", env.Error.Details)
	}
}

func TestSkillsLint_MissingDir_IsUsageOrEmptyResult(t *testing.T) {
	// A directory with no skills/ subdirectory at all is not an error —
	// it lints zero skills, matching skills.DiscoverSkills's own
	// tolerance.
	dir := t.TempDir()
	stdout, _, exitCode := runRoot(t, "skills", "lint", dir)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
}

func TestSkillsInstall_CopiesFromValidFixture(t *testing.T) {
	from := filepath.Join("..", "..", "testdata", "skills", "valid")
	into := t.TempDir()

	stdout, _, exitCode := runRoot(t, "skills", "install", "--from", from, "--into", into)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	if _, err := os.Stat(filepath.Join(into, "develop", "SKILL.md")); err != nil {
		t.Errorf("develop/SKILL.md not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(into, "run-mutation", "references", "tdd.md")); err != nil {
		t.Errorf("run-mutation/references/tdd.md not installed: %v", err)
	}
}

func TestSkillsInstall_MissingFlags_IsUsageError(t *testing.T) {
	stdout, _, exitCode := runRoot(t, "skills", "install", "--from", t.TempDir())
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", exitCode, stdout.String())
	}
}
