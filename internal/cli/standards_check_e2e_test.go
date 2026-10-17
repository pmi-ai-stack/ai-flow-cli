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

// TestStandardsCheckFindsSecretInMarkdown runs the directory form with
// a real gitleaks against a repository whose README carries an
// AWS-shaped access key. The reference pack's secrets check must report
// it: Markdown is not exempt (security review 2026-09-27 S5).
//
// Needs a real gitleaks. Skipped when there is none, unless
// AI_FLOW_E2E_REQUIRE_TOOLS=1 (CI's required job sets it).
func TestStandardsCheckFindsSecretInMarkdown(t *testing.T) {
	gitleaks, ok := exec.Which("gitleaks")
	if !ok {
		if os.Getenv("AI_FLOW_E2E_REQUIRE_TOOLS") == "1" {
			t.Fatal("gitleaks is not on PATH and AI_FLOW_E2E_REQUIRE_TOOLS=1")
		}
		t.Skip("gitleaks not on PATH")
	}
	bin := t.TempDir()
	if err := os.Symlink(gitleaks, filepath.Join(bin, "gitleaks")); err != nil {
		t.Fatalf("symlink gitleaks: %v", err)
	}

	repo := filepath.Join(t.TempDir(), "docs-repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	readme := "# Setup\n\nexport AWS_ACCESS_KEY_ID=AKIAQWERTYUIOPASDFGH\n"
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(readme), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", bin)
	t.Chdir(repo)

	stdout, _, ec := runRoot(t, "standards", "check", ".")
	if ec != 6 {
		t.Fatalf("exitCode = %d, want 6 (invalid_result)\n%s", ec, stdout.String())
	}
	var env dirCheckEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error == nil || !slices.ContainsFunc(env.Error.Details, func(d string) bool {
		return strings.HasPrefix(d, "org.secrets: README.md")
	}) {
		t.Errorf("no org.secrets finding on README.md:\n%s", stdout.String())
	}
}

// TestStandardsCheckSARIFEndToEnd is the README's CI command with
// --sarif, run for real against conftest: the file holds the two
// policy violations at main.tf as errors, plus the two checks whose
// tools are not on PATH (the must secrets check as an error, the should
// checkov scan as a warning), located at the repository root.
//
// Needs a real conftest. Skipped when there is none, unless
// AI_FLOW_E2E_REQUIRE_TOOLS=1 (CI's required job sets it).
func TestStandardsCheckSARIFEndToEnd(t *testing.T) {
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
	args := readmeCICommand(t)
	repo := stageTerraformE2E(t)
	sarif := filepath.Join(t.TempDir(), "ai-flow.sarif")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", bin)
	t.Chdir(repo)

	stdout, _, ec := runRoot(t, append(args, "--sarif", sarif)...)
	if ec != 6 {
		t.Fatalf("exitCode = %d, want 6\n%s", ec, stdout.String())
	}
	var got []string
	for _, r := range readSARIF(t, sarif).Runs[0].Results {
		got = append(got, r.RuleID+" "+r.Level+" "+r.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	}
	want := []string{
		"infra.terraform.scan warning .",
		"org.secrets error .",
		"infra.terraform.policy error main.tf",
		"infra.terraform.policy error main.tf",
	}
	if !slices.Equal(got, want) {
		t.Errorf("results = %q\nwant      %q", got, want)
	}
}
