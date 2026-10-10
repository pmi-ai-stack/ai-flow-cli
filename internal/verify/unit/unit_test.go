package unit_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/unit"
)

func TestRunGoPass(t *testing.T) {
	data, err := os.ReadFile("testdata/go/pass.jsonl")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).On("go", []string{"test", "-json", "./..."}, exec.Output{Stdout: string(data)})
	repo := detect.Repo{Name: "fixture", LocalPath: t.TempDir()}
	m := detect.RunnerMatch{Runner: "go", Command: "go", Args: []string{"test", "-json", "./..."}}

	r, err := unit.Run(context.Background(), repo, m, s, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Repo != "fixture" || r.Runner != "go" || r.Total != 1 || r.Passed != 1 {
		t.Errorf("Run = %+v", r)
	}
}

func TestRunGoFailingSuiteStillParses(t *testing.T) {
	data, err := os.ReadFile("testdata/go/fail.jsonl")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).OnFailWithOutput(
		"go", []string{"test", "-json", "./..."},
		exec.Output{Stdout: string(data), ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "go test: exit status 1"},
	)
	repo := detect.Repo{Name: "fixture", LocalPath: t.TempDir()}
	m := detect.RunnerMatch{Runner: "go", Command: "go", Args: []string{"test", "-json", "./..."}}

	r, err := unit.Run(context.Background(), repo, m, s, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v, want a failing suite to parse cleanly, not error", err)
	}
	if r.Total != 2 || r.Failed != 1 {
		t.Errorf("Run = %+v, want total=2 failed=1", r)
	}
}

func TestRunExecNotAllowedPropagates(t *testing.T) {
	s := exectest.New(t).OnFail("go", []string{"test", "-json", "./..."}, exec.NotAllowed([]string{"go", "test", "-json", "./..."}))
	repo := detect.Repo{Name: "fixture", LocalPath: t.TempDir()}
	m := detect.RunnerMatch{Runner: "go", Command: "go", Args: []string{"test", "-json", "./..."}}

	_, err := unit.Run(context.Background(), repo, m, s, time.Minute)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "exec_not_allowed" {
		t.Fatalf("Run error = %v, want exec_not_allowed", err)
	}
}

func TestRunJest(t *testing.T) {
	data, err := os.ReadFile("testdata/jest/fail.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).OnFailWithOutput(
		"npx", []string{"jest", "--ci", "--json"},
		exec.Output{Stdout: string(data), ExitCode: 1},
		&envelope.Error{Code: "tool_failed"},
	)
	repo := detect.Repo{Name: "web", LocalPath: t.TempDir()}
	m := detect.RunnerMatch{Runner: "jest", Command: "npx", Args: []string{"jest", "--ci", "--json"}}

	r, err := unit.Run(context.Background(), repo, m, s, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Total != 2 || r.Failed != 1 {
		t.Errorf("Run = %+v, want total=2 failed=1", r)
	}
}

// pytestFakeExecer simulates pytest's real behaviour: it writes its
// JUnit report to the file Run's own --junitxml=<path> argument names,
// then exits non-zero when the fixture is a failing suite — a side
// effect exectest.Scripted cannot express (it only ever returns a
// canned Output, never touches the file system), so this test needs
// its own small fake.
type pytestFakeExecer struct {
	t        *testing.T
	fixture  string
	exitCode int
}

func (f pytestFakeExecer) Run(_ context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	if err := exectest.Refused(tool, args); err != nil {
		return exec.Output{}, err // what the real Runner answers (phase 54)
	}
	f.t.Helper()
	if tool != "pytest" {
		f.t.Fatalf("unexpected tool %q", tool)
	}
	data, err := os.ReadFile(f.fixture)
	if err != nil {
		f.t.Fatalf("ReadFile fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(opts.Dir, ".ai-flow-pytest-report.xml"), data, 0o600); err != nil {
		f.t.Fatalf("WriteFile report: %v", err)
	}
	if f.exitCode != 0 {
		return exec.Output{ExitCode: f.exitCode}, &envelope.Error{Code: "tool_failed", Message: "pytest: exit status " + string(rune('0'+f.exitCode))}
	}
	return exec.Output{}, nil
}

func TestRunPytestPass(t *testing.T) {
	fake := pytestFakeExecer{t: t, fixture: "testdata/pytest/pass.xml"}
	repo := detect.Repo{Name: "svc", LocalPath: t.TempDir()}
	m := detect.RunnerMatch{Runner: "pytest", Command: "pytest"}

	r, err := unit.Run(context.Background(), repo, m, fake, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Total != 1 || r.Passed != 1 {
		t.Errorf("Run = %+v, want total=1 passed=1", r)
	}
	if _, statErr := os.Stat(filepath.Join(repo.LocalPath, ".ai-flow-pytest-report.xml")); !os.IsNotExist(statErr) {
		t.Error("pytest report file was not cleaned up after Run")
	}
}

func TestRunPytestFail(t *testing.T) {
	fake := pytestFakeExecer{t: t, fixture: "testdata/pytest/fail.xml", exitCode: 1}
	repo := detect.Repo{Name: "svc", LocalPath: t.TempDir()}
	m := detect.RunnerMatch{Runner: "pytest", Command: "pytest"}

	r, err := unit.Run(context.Background(), repo, m, fake, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Total != 2 || r.Failed != 1 {
		t.Errorf("Run = %+v, want total=2 failed=1", r)
	}
}
