package cli

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// readmeCICommand is the argv README.md's "Use in CI" section gives,
// without the leading "ai-flow" — read from the README itself, so the
// documented command and the tested one cannot drift apart.
func readmeCICommand(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("open README.md: %v", err)
	}
	defer func() { _ = f.Close() }()

	inSection := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			inSection = line == "## Use in CI"
			continue
		}
		if inSection && strings.HasPrefix(line, "ai-flow ") {
			return strings.Fields(line)[1:]
		}
	}
	t.Fatal(`README.md has no "## Use in CI" section with an "ai-flow …" command`)
	return nil
}

// TestStandardsCheckReadmeCommandEndToEnd is the documented CI use case
// run for real (item 2.3): the README's exact command, the real
// exec.Runner, a terraform repository, a HOME with no ~/.ai-flow in it.
// PATH holds conftest and nothing else, which makes the result the same
// on every machine and exercises both halves of "never silently pass":
// the policy check reports its two violations, and gitleaks — the must
// secrets check — reports that it could not run.
//
// Needs a real conftest. Skipped when there is none, unless
// AI_FLOW_E2E_REQUIRE_TOOLS=1 (CI's required job sets it).
func TestStandardsCheckReadmeCommandEndToEnd(t *testing.T) {
	conftest, ok := exec.Which("conftest")
	if !ok {
		if os.Getenv("AI_FLOW_E2E_REQUIRE_TOOLS") == "1" {
			t.Fatal("conftest is not on PATH and AI_FLOW_E2E_REQUIRE_TOOLS=1")
		}
		t.Skip("conftest not on PATH")
	}
	bin := t.TempDir()
	if err := os.Symlink(conftest, filepath.Join(bin, "conftest")); err != nil {
		t.Fatalf("symlink conftest: %v", err)
	}

	args := readmeCICommand(t) // before Chdir: the path is relative to this package
	repo := stageTerraformE2E(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", bin)
	t.Chdir(repo)

	stdout, _, ec := runRoot(t, args...)
	if ec != 6 {
		t.Fatalf("exitCode = %d, want 6 (invalid_result)\n%s", ec, stdout.String())
	}

	var env dirCheckEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	want := []string{
		`infra.terraform.policy: main.tf: terraform resource google_storage_bucket.LogsBucket: local name must be snake_case (lowercase letters, digits, underscores)`,
		`infra.terraform.policy: main.tf: terraform resource type "google_storage_bucket" is not from an approved provider (allowed prefixes: aws_, random_, null_, local_)`,
		`org.secrets: infra-repo: gitleaks is not installed (not on PATH); check org.secrets did not run`,
	}
	if env.Error == nil || !slices.Equal(env.Error.Details, want) {
		t.Errorf("details = %q\nwant      %q", env.Error, want)
	}
}
