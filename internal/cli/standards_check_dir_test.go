package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// toolFunc is an Execer answering by tool name, for check runners whose
// argv carries a temp path a test cannot script verbatim (conftest's
// materialized policy directory). Every call is recorded.
type toolFunc struct {
	answer func(tool string, args []string) (exec.Output, error)
	calls  []string
}

func (f *toolFunc) Run(_ context.Context, tool string, args []string, _ exec.RunOpts) (exec.Output, error) {
	if err := exectest.Refused(tool, args); err != nil {
		return exec.Output{}, err // what the real Runner answers (phase 54)
	}
	f.calls = append(f.calls, tool)
	return f.answer(tool, args)
}

func useExecer(t *testing.T, x exec.Execer) {
	t.Helper()
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })
}

// stageTerraformE2E copies testdata/repos/terraform-e2e into a temp
// directory, giving main.tf.fixture its real name — stored under
// another name so this repository's own `standards check .` does not
// flag a fixture that violates the reference policy on purpose.
func stageTerraformE2E(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "infra-repo")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "repos", "terraform-e2e"))); err != nil {
		t.Fatalf("CopyFS: %v", err)
	}
	if err := os.Rename(filepath.Join(dir, "main.tf.fixture"), filepath.Join(dir, "main.tf")); err != nil {
		t.Fatalf("rename fixture: %v", err)
	}
	return dir
}

const conftestTwoFailures = `[{"filename":"main.tf","namespace":"main","failures":[` +
	`{"msg":"terraform resource google_storage_bucket.LogsBucket: local name must be snake_case"},` +
	`{"msg":"terraform resource type \"google_storage_bucket\" is not from an approved provider"}]}]`

type dirCheckEnvelope struct {
	OK     bool `json:"ok"`
	Result *struct {
		Dir      string `json:"dir"`
		Open     int    `json:"open"`
		Findings []struct {
			CheckID  string `json:"check_id"`
			Severity string `json:"severity"`
			Tool     string `json:"tool"`
			Repo     string `json:"repo"`
			Target   string `json:"target"`
			Status   string `json:"status"`
			Message  string `json:"message"`
		} `json:"findings"`
		Skipped []skippedCheck `json:"skipped"`
	} `json:"result"`
	Error *struct {
		Code    string   `json:"code"`
		Message string   `json:"message"`
		Details []string `json:"details"`
	} `json:"error"`
	Warnings []string `json:"warnings"`
}

func runDirCheck(t *testing.T, wantExit int, args ...string) dirCheckEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, args...)
	if ec != wantExit {
		t.Fatalf("%v: exitCode = %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	var env dirCheckEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	return env
}

// TestStandardsCheckDirRunsTerraformPolicyWithoutConfig is item 2.1 and
// 2.2 together, the way an adopter's CI meets them: no config.json, no
// AGENTS.md, a terraform repository — and the policy check runs. Before
// phase 33 this was missing_artifact, then missing_repo_instructions,
// then (with both worked around) zero findings and exit 0.
func TestStandardsCheckDirRunsTerraformPolicyWithoutConfig(t *testing.T) {
	repo := stageTerraformE2E(t)
	x := &toolFunc{answer: func(tool string, _ []string) (exec.Output, error) {
		switch tool {
		case "conftest":
			return exec.Output{Stdout: conftestTwoFailures, ExitCode: 1}, &exitOneToolFailed
		case "checkov":
			return exec.Output{Stdout: `{"results":{"failed_checks":[]}}`}, nil
		}
		return exec.Output{}, nil // gitleaks: no report file, no leaks
	}}
	useExecer(t, x)

	env := runDirCheck(t, 6, "standards", "check", repo, "--home", filepath.Join(t.TempDir(), "no-such-home"))

	if env.Error.Code != "invalid_result" || len(env.Error.Details) != 2 {
		t.Fatalf("error = %+v, want invalid_result with the two policy failures", env.Error)
	}
	for _, d := range env.Error.Details {
		if !strings.HasPrefix(d, "infra.terraform.policy: main.tf: ") {
			t.Errorf("detail %q does not name infra.terraform.policy on main.tf", d)
		}
	}
	if !slices.Contains(x.calls, "conftest") || !slices.Contains(x.calls, "checkov") {
		t.Errorf("calls = %v, want conftest and checkov", x.calls)
	}
	if len(env.Warnings) != 2 ||
		!strings.Contains(env.Warnings[0], "no config.json") ||
		!strings.Contains(env.Warnings[1], "infra-repo has neither AGENTS.md nor CLAUDE.md") {
		t.Errorf("warnings = %q, want the built-in-defaults and the instructions warnings", env.Warnings)
	}
}

// exitOneToolFailed is a conftest "found violations" exit — tool_failed
// with its report on stdout.
var exitOneToolFailed = envelope.Error{Code: "tool_failed", Message: "conftest test: exit status 1"}

// TestStandardsCheckDirMissingToolIsAFinding is item 2.2.4: a check
// whose tool is not installed reports so at the check's severity. A
// must check (org.secrets/gitleaks) blocks; a should check
// (infra.terraform.scan/checkov) only reports.
func TestStandardsCheckDirMissingToolIsAFinding(t *testing.T) {
	repo := stageTerraformE2E(t)
	writeFileTest(t, filepath.Join(repo, "AGENTS.md"), "# infra\n")
	useExecer(t, &toolFunc{answer: func(tool string, _ []string) (exec.Output, error) {
		if tool == "conftest" {
			return exec.Output{Stdout: `[{"filename":"main.tf","failures":[]}]`}, nil
		}
		return exec.Output{}, exec.NotFound(tool)
	}})

	env := runDirCheck(t, 6, "standards", "check", repo, "--home", t.TempDir())
	want := []string{"org.secrets: infra-repo: gitleaks is not installed (not on PATH); check org.secrets did not run"}
	if !slices.Equal(env.Error.Details, want) {
		t.Errorf("details = %q, want %q", env.Error.Details, want)
	}

	// Only the should check left missing: exit 0, reported as a finding.
	useExecer(t, &toolFunc{answer: func(tool string, _ []string) (exec.Output, error) {
		switch tool {
		case "conftest":
			return exec.Output{Stdout: `[{"filename":"main.tf","failures":[]}]`}, nil
		case "checkov":
			return exec.Output{}, exec.NotFound(tool)
		}
		return exec.Output{}, nil
	}})
	env = runDirCheck(t, 0, "standards", "check", repo, "--home", t.TempDir())
	if env.Result.Open != 1 || len(env.Result.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the missing-checkov one", env.Result.Findings)
	}
	f := env.Result.Findings[0]
	if f.CheckID != "infra.terraform.scan" || f.Severity != "should" || f.Tool != "checkov" ||
		f.Target != "infra-repo:." || f.Status != "open" || !strings.Contains(f.Message, "checkov is not installed") {
		t.Errorf("finding = %+v", f)
	}
}

// TestStandardsCheckDirReportsRenderChecksAsSkipped: the directory form
// renders nothing, so `input: render` checks cannot run there — and
// say so, instead of looking like a clean pass.
func TestStandardsCheckDirReportsRenderChecksAsSkipped(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "wf")
	writeFileTest(t, filepath.Join(repo, "AGENTS.md"), "# wf\n")
	writeFileTest(t, filepath.Join(repo, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps: [{run: echo}]\n")
	useExecer(t, &toolFunc{answer: func(string, []string) (exec.Output, error) { return exec.Output{}, nil }})

	env := runDirCheck(t, 0, "standards", "check", repo, "--home", t.TempDir())
	var got []string
	for _, s := range env.Result.Skipped {
		got = append(got, s.CheckID+" "+s.Target)
	}
	want := []string{"infra.github-actions.policy wf:.github/workflows/ci.yml", "infra.github-actions.scan wf:.github/workflows/ci.yml"}
	if !slices.Equal(got, want) {
		t.Errorf("skipped = %v, want %v", got, want)
	}
}

func TestStandardsCheckDirRequireInstructions(t *testing.T) {
	repo := stageTerraformE2E(t)
	useExecer(t, &toolFunc{answer: func(string, []string) (exec.Output, error) { return exec.Output{}, nil }})
	env := runDirCheck(t, 4, "standards", "check", repo, "--home", t.TempDir(), "--require-instructions")
	if env.Error.Code != "missing_repo_instructions" {
		t.Errorf("error = %+v, want missing_repo_instructions", env.Error)
	}
}

// TestStandardsCheckDirExplicitConfigMustExist: the built-in defaults
// stand in only for the implicit <home>/config.json — a --config the
// caller named and that does not exist is still an error.
func TestStandardsCheckDirExplicitConfigMustExist(t *testing.T) {
	repo := stageTerraformE2E(t)
	env := runDirCheck(t, 3, "standards", "check", repo, "--config", filepath.Join(t.TempDir(), "missing.json"))
	if env.Error.Code != "missing_artifact" {
		t.Errorf("error = %+v, want missing_artifact", env.Error)
	}
}

// writeMiniPack writes a one-check pack: gitleaks only.
func writeMiniPack(t *testing.T, dir string) {
	t.Helper()
	writeFileTest(t, filepath.Join(dir, "standards.json"), `{
  "schema_version": 1, "id": "mini", "version": "1.0.0", "sections": [],
  "checks": [{ "id": "mini.secrets", "kind": "secrets", "tool": "gitleaks", "severity": "must" }]
}
`)
}

// TestStandardsCheckDirPackFlag: --pack replaces the org layer, as a
// directory or as <source>@<ref> (synced into a temporary home — a
// local source here, so no network is needed).
func TestStandardsCheckDirPackFlag(t *testing.T) {
	repo := stageTerraformE2E(t)
	pack := filepath.Join(t.TempDir(), "mini-pack")
	writeMiniPack(t, pack)

	for _, arg := range []string{pack, pack + "@v1"} {
		x := &toolFunc{answer: func(string, []string) (exec.Output, error) { return exec.Output{}, nil }}
		useExecer(t, x)
		env := runDirCheck(t, 0, "standards", "check", repo, "--home", t.TempDir(), "--pack", arg)
		if !env.OK || !slices.Equal(x.calls, []string{"gitleaks"}) {
			t.Errorf("--pack %s: calls = %v, want only the mini pack's gitleaks (no reference-pack terraform checks)", arg, x.calls)
		}
	}
}

func TestStandardsCheckPackFlagUsageErrors(t *testing.T) {
	repo := stageTerraformE2E(t)
	for _, arg := range []string{"no-such-dir", "https://example.com/pack.git@", "@v1"} {
		env := runDirCheck(t, 2, "standards", "check", repo, "--home", t.TempDir(), "--pack", arg)
		if env.Error.Code != "usage" {
			t.Errorf("--pack %q: error = %+v, want usage", arg, env.Error)
		}
	}

	home, _ := standardsTicket(t, "PROJ-9")
	env := runDirCheck(t, 2, "standards", "check", "PROJ-9", "--home", home, "--pack", repo)
	if env.Error.Code != "usage" {
		t.Errorf("--pack with a ticket key: error = %+v, want usage", env.Error)
	}
}
