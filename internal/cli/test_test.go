package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// pytestWritingExecer wraps a *exectest.Scripted with pytest's own
// side effect (writing its report to the --junitxml path) —
// exectest.Scripted has no side-effect mechanism (see
// internal/verify/unit's own tests, which need the same wrapper for
// the same reason). Every non-pytest call still goes through the
// scripted double.
type pytestWritingExecer struct {
	scripted *exectest.Scripted
	t        *testing.T
	fixture  string
}

func (p *pytestWritingExecer) Run(ctx context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	if tool == "pytest" {
		data, err := os.ReadFile(p.fixture)
		if err != nil {
			p.t.Fatalf("ReadFile fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(opts.Dir, ".ai-flow-pytest-report.xml"), data, 0o600); err != nil {
			p.t.Fatalf("WriteFile report: %v", err)
		}
		return exec.Output{}, nil
	}
	return p.scripted.Run(ctx, tool, args, opts)
}

// usePytestWritingExecer swaps newExecer for a pytestWritingExecer
// wrapping s, restoring the real constructor afterward — mirrors
// useScriptedExecer's own shape (git.go's newExecer is the same
// injectable package var).
func usePytestWritingExecer(t *testing.T, s *exectest.Scripted, fixture string) {
	t.Helper()
	orig := newExecer
	p := &pytestWritingExecer{scripted: s, t: t, fixture: fixture}
	newExecer = func() exec.Execer { return p }
	t.Cleanup(func() { newExecer = orig })
}

func TestTestRunAcrossThreeFixtureRepos(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{
		{"go-svc", fixtureRepoPath(t, "go")},
		{"web", fixtureRepoPath(t, "typescript")},
		{"api", fixtureRepoPath(t, "python")},
	}, nil)

	s := newScriptedExecer(t)
	s.On("go", []string{"test", "-json", "./..."}, exec.Output{Stdout: `{"Action":"run","Test":"TestX"}
{"Action":"pass","Test":"TestX"}
`})
	s.OnFailWithOutput(
		"npx", []string{"jest", "--ci", "--json"},
		exec.Output{Stdout: `{"numTotalTests":1,"numPassedTests":0,"numFailedTests":1,"testResults":[{"assertionResults":[{"fullName":"t","status":"failed","failureMessages":["boom"]}]}]}`, ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	usePytestWritingExecer(t, s, filepath.Join("..", "verify", "unit", "testdata", "pytest", "fail.xml"))

	stdout, _, exitCode := runRoot(t, "test", "run", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result testResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Repositories) != 3 {
		t.Fatalf("Repositories = %+v, want 3 entries", env.Result.Repositories)
	}
	byRepo := map[string]testResultsRepo{}
	for _, r := range env.Result.Repositories {
		byRepo[r.Repo] = r
	}
	if got := byRepo["go-svc"]; got.Runner != "go" || got.Passed != 1 {
		t.Errorf("go-svc = %+v", got)
	}
	if got := byRepo["web"]; got.Runner != "jest" || got.Failed != 1 {
		t.Errorf("web = %+v", got)
	}
	if got := byRepo["api"]; got.Runner != "pytest" || got.Total != 2 || got.Failed != 1 {
		t.Errorf("api = %+v, want total=2 failed=1 (testdata/pytest/fail.xml)", got)
	}
}

func TestTestRunRepoFlagNarrows(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{
		{"go-svc", fixtureRepoPath(t, "go")},
		{"web", fixtureRepoPath(t, "typescript")},
	}, nil)

	s := newScriptedExecer(t)
	s.On("go", []string{"test", "-json", "./..."}, exec.Output{Stdout: `{"Action":"run","Test":"TestX"}
{"Action":"pass","Test":"TestX"}
`})
	useScriptedExecer(t, s)

	stdout, _, exitCode := runRoot(t, "test", "run", "PROJ-1", "--repo", "go-svc", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result testResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Repositories) != 1 || env.Result.Repositories[0].Repo != "go-svc" {
		t.Errorf("Repositories = %+v, want only go-svc", env.Result.Repositories)
	}
}

func TestTestRunRepoFlagUnknownRepo(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"go-svc", fixtureRepoPath(t, "go")}}, nil)

	stdout, _, exitCode := runRoot(t, "test", "run", "PROJ-1", "--repo", "nope", "--home", home)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", exitCode, stdout.String())
	}
}

func TestTestRunNoRunnerDetectedIsWarnedNotFailed(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"empty", t.TempDir()}}, nil)

	stdout, _, exitCode := runRoot(t, "test", "run", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result   testResultsArtifact `json:"result"`
		Warnings []string            `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Repositories) != 0 {
		t.Errorf("Repositories = %+v, want empty (no runner detected)", env.Result.Repositories)
	}
	if len(env.Warnings) != 1 {
		t.Errorf("Warnings = %v, want one entry", env.Warnings)
	}
}

func TestTestRunSimulateWritesNothing(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"go-svc", fixtureRepoPath(t, "go")}}, nil)

	stdout, _, exitCode := runRoot(t, "test", "run", "PROJ-1", "--simulate", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(home, "PROJ-1", "test-results.json")); err == nil {
		t.Error("--simulate must not write test-results.json")
	}
}
