package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func e2eReportFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "verify", "e2e", "testdata", "playwright", "report.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func TestE2ERunFirstCallHasNoNewCases(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", fixtureRepoPath(t, "typescript")}}, nil)

	s := newScriptedExecer(t)
	s.OnFailWithOutput(
		"npx", []string{"playwright", "test", "--reporter=json"},
		exec.Output{Stdout: e2eReportFixture(t), ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	useScriptedExecer(t, s)

	stdout, _, exitCode := runRoot(t, "e2e", "run", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result e2eResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.Result.Total != 3 || env.Result.Failed != 1 {
		t.Errorf("Result = %+v, want total=3 failed=1", env.Result)
	}
	if len(env.Result.NewCases) != 0 {
		t.Errorf("NewCases = %v, want empty on the first call", env.Result.NewCases)
	}
	if len(env.Result.CaseNames) != 3 {
		t.Errorf("CaseNames = %v, want 3 entries", env.Result.CaseNames)
	}
}

func TestE2ERunSecondCallComparesAgainstFirst(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", fixtureRepoPath(t, "typescript")}}, nil)

	s := newScriptedExecer(t)
	s.OnFailWithOutput(
		"npx", []string{"playwright", "test", "--reporter=json"},
		exec.Output{Stdout: e2eReportFixture(t), ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	useScriptedExecer(t, s)

	// First call (simulating the base-ref run) — records a baseline.
	if _, _, ec := runRoot(t, "e2e", "run", "PROJ-1", "--home", home); ec != 0 {
		t.Fatalf("first e2e run: exitCode = %d", ec)
	}
	// Second call (simulating the branch-ref run, identical report) —
	// every case was already in the baseline, so nothing is new.
	stdout, _, exitCode := runRoot(t, "e2e", "run", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result e2eResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.NewCases) != 0 {
		t.Errorf("NewCases = %v, want empty (identical suite both times)", env.Result.NewCases)
	}
}

func TestE2ERunNoSuiteDetected(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"go-svc", fixtureRepoPath(t, "go")}}, nil)

	stdout, _, exitCode := runRoot(t, "e2e", "run", "PROJ-1", "--home", home)
	if exitCode != 4 {
		t.Fatalf("exitCode = %d, want 4 (tool_missing)\n%s", exitCode, stdout.String())
	}
}

func TestE2ERunCaseFlag(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", fixtureRepoPath(t, "typescript")}}, nil)

	s := newScriptedExecer(t)
	s.On("npx", []string{"playwright", "test", "--reporter=json", "--grep", "login"}, exec.Output{Stdout: e2eReportFixture(t)})
	useScriptedExecer(t, s)

	stdout, _, exitCode := runRoot(t, "e2e", "run", "PROJ-1", "--case", "login", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
}
