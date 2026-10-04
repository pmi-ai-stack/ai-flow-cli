package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// scriptEmbeddedGoChecks scripts every subprocess the embedded
// reference pack's checks fire against a go.mod-only repository with
// no known violations: `go.format` (gofmt), `go.lint`
// (golangci-lint) and `org.secrets` (gitleaks) all apply to every
// stage (none narrows applies_to.stages), so both a --stage checks
// and a --stage ship run make the same three calls — this double
// registers each once and both runs reuse it.
func scriptEmbeddedGoChecks(t *testing.T, golangciReport string) *exectest.Scripted {
	t.Helper()
	golangciConfig := filepath.Join(os.TempDir(), "ai-flow-check-config-go.lint.yml")
	secretsConfig := filepath.Join(os.TempDir(), "ai-flow-check-config-org.secrets.toml")
	secretsReport := filepath.Join(os.TempDir(), "ai-flow-check-report-org.secrets.json")
	t.Cleanup(func() {
		_ = os.Remove(golangciConfig)
		_ = os.Remove(secretsConfig)
		_ = os.Remove(secretsReport)
	})

	return exectest.New(t).
		On("gofmt", []string{"-l", "."}, exec.Output{}).
		On("golangci-lint", []string{"run", "--output.json.path", "stdout", "--show-stats=false", "--path-mode", "abs", "--config", golangciConfig, "./..."}, exec.Output{Stdout: golangciReport}).
		On("gitleaks", []string{
			"detect", "--source", ".", "--no-git", "--report-format", "json",
			"--report-path", secretsReport, "--exit-code", "0", "--config", secretsConfig,
		}, exec.Output{})
}

type checksEnvelope struct {
	OK     bool `json:"ok"`
	Result struct {
		Key      string `json:"key"`
		Dir      string `json:"dir"`
		Stage    string `json:"stage"`
		Fixed    int    `json:"fixed"`
		Open     int    `json:"open"`
		Waived   int    `json:"waived"`
		Findings []struct {
			CheckID  string `json:"check_id"`
			Severity string `json:"severity"`
			Stage    string `json:"stage"`
			Status   string `json:"status"`
			File     string `json:"file"`
		} `json:"findings"`
	} `json:"result"`
	Error *struct {
		Code    string   `json:"code"`
		Message string   `json:"message"`
		Details []string `json:"details"`
	} `json:"error"`
}

func runChecks(t *testing.T, wantExit int, args ...string) checksEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, args...)
	if ec != wantExit {
		t.Fatalf("%v: exitCode = %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	var env checksEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("%v: stdout is not valid JSON: %v\n%s", args, err, stdout.String())
	}
	return env
}

func TestStandardsCheckTicketCleanRepoNoFindings(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	env := runChecks(t, 0, "standards", "check", "PROJ-1", "--home", home)
	if env.Result.Key != "PROJ-1" || env.Result.Stage != "checks" {
		t.Fatalf("result = %+v", env.Result)
	}
	if env.Result.Open != 0 {
		t.Fatalf("Open = %d, want 0", env.Result.Open)
	}

	data, err := os.ReadFile(filepath.Join(home, "PROJ-1", "checks-results.json"))
	if err != nil {
		t.Fatalf("read checks-results.json: %v", err)
	}
	var onDisk struct {
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("decode checks-results.json: %v", err)
	}
	for _, f := range onDisk.Findings {
		if f["status"] == "open" {
			t.Errorf("unexpected open finding on disk: %+v", f)
		}
	}
}

func TestStandardsCheckPerStageReplacement(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-2")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[{"Text":"unused var","Pos":{"Filename":"main.go","Line":3},"FromLinter":"unused"}]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	checksEnv := runChecks(t, 0, "standards", "check", "PROJ-2", "--home", home)
	if checksEnv.Result.Open == 0 {
		t.Fatalf("expected at least one open finding from the seeded golangci-lint issue, got %+v", checksEnv.Result)
	}

	shipEnv := runChecks(t, 0, "standards", "check", "PROJ-2", "--stage", "ship", "--home", home)
	_ = shipEnv

	data, err := os.ReadFile(filepath.Join(home, "PROJ-2", "checks-results.json"))
	if err != nil {
		t.Fatalf("read checks-results.json: %v", err)
	}
	var onDisk struct {
		Findings []struct {
			CheckID string `json:"check_id"`
			Stage   string `json:"stage"`
			Status  string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("decode checks-results.json: %v", err)
	}

	var sawChecksStageGoLint bool
	for _, f := range onDisk.Findings {
		if f.CheckID == "go.lint" && f.Stage == "checks" && f.Status == "open" {
			sawChecksStageGoLint = true
		}
	}
	if !sawChecksStageGoLint {
		t.Errorf("the --stage checks go.lint finding was erased by the --stage ship run; findings = %+v", onDisk.Findings)
	}
}

func TestStandardsCheckMustFindingBlocksStageComplete(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-3")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[{"Text":"unused var","Pos":{"Filename":"main.go","Line":3},"FromLinter":"unused"}]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	runChecks(t, 0, "standards", "check", "PROJ-3", "--home", home)

	if _, _, ec := runRoot(t, "stage", "start", "PROJ-3", "checks", "--home", home); ec != 0 {
		t.Fatalf("stage start checks: exitCode = %d", ec)
	}
	_, _, ec := runRoot(t, "stage", "complete", "PROJ-3", "checks", "--home", home)
	if ec != 3 {
		t.Fatalf("stage complete checks with an open must finding: exitCode = %d, want 3 (must_findings_open)", ec)
	}
}

func TestStandardsCheckDirFormFailsOnOpenMust(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "AGENTS.md"), "# repo\n")
	writeFileTest(t, filepath.Join(repoDir, "go.mod"), "module example.com/acme\n\ngo 1.27\n")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[{"Text":"unused var","Pos":{"Filename":"main.go","Line":3},"FromLinter":"unused"}]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	stdout, _, ec := runRoot(t, "standards", "check", repoDir, "--home", home)
	if ec != 6 {
		t.Fatalf("standards check <dir> with an open must finding: exitCode = %d, want 6 (invalid_result)\n%s", ec, stdout.String())
	}
	var env checksEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error == nil || env.Error.Code != "invalid_result" || len(env.Error.Details) == 0 {
		t.Fatalf("error = %+v, want invalid_result with details", env.Error)
	}
}

func TestStandardsCheckDirFormCleanExitsZero(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "AGENTS.md"), "# repo\n")
	writeFileTest(t, filepath.Join(repoDir, "go.mod"), "module example.com/acme\n\ngo 1.27\n")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	env := runChecks(t, 0, "standards", "check", repoDir, "--home", home)
	if env.Result.Dir != repoDir {
		t.Errorf("Dir = %q, want %q", env.Result.Dir, repoDir)
	}
	if env.Result.Open != 0 {
		t.Fatalf("Open = %d, want 0", env.Result.Open)
	}
}

func TestStandardsCheckUsageBadStage(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-4")
	_, _, ec := runRoot(t, "standards", "check", "PROJ-4", "--stage", "implement", "--home", home)
	if ec != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)", ec)
	}
}

// The directory form runs stage `checks` only. Accepting --stage ship
// there would let a CI author believe ship-scoped checks ran.
func TestStandardsCheckDirFormRejectsShipStage(t *testing.T) {
	home := t.TempDir()
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "AGENTS.md"), "# repo\n")

	stdout, _, ec := runRoot(t, "standards", "check", repoDir, "--stage", "ship", "--home", home)
	if ec != 2 {
		t.Fatalf("standards check <dir> --stage ship: exitCode = %d, want 2 (usage)\n%s", ec, stdout.String())
	}
	var env checksEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error == nil || env.Error.Code != "usage" || !strings.Contains(env.Error.Message, "--stage") {
		t.Fatalf("error = %+v, want usage naming --stage", env.Error)
	}
}

func TestStandardsCheckDirFormAcceptsExplicitChecksStage(t *testing.T) {
	home := t.TempDir()
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "AGENTS.md"), "# repo\n")
	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	runChecks(t, 0, "standards", "check", repoDir, "--stage", "checks", "--home", home)
}
