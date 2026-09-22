package mutmut

import (
	"context"
	"os"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func TestParsePass(t *testing.T) {
	data, err := os.ReadFile("testdata/pass.xml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.Killed != 2 || len(got.Survivors) != 0 {
		t.Errorf("Parse(pass) = %+v", got)
	}
}

func TestParseBelowThreshold(t *testing.T) {
	data, err := os.ReadFile("testdata/below_threshold.xml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 || got.Killed != 2 || len(got.Survivors) != 1 {
		t.Errorf("Parse(below_threshold) = %+v", got)
	}
	if got.Survivors[0].ID != "2" || got.Survivors[0].File != "pkg/add.py" || got.Survivors[0].Line != 9 {
		t.Errorf("Survivors[0] = %+v", got.Survivors[0])
	}
}

func TestParseMalformed(t *testing.T) {
	if _, err := Parse([]byte("not xml")); err == nil {
		t.Error("Parse(malformed) = nil error, want one")
	}
}

type fakeExecer struct {
	t          *testing.T
	junitxml   string
	calls      []string
	failOnRun  bool
	failOnJXML bool
}

func (f *fakeExecer) Run(_ context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	f.t.Helper()
	if tool != "mutmut" {
		f.t.Fatalf("unexpected tool %q", tool)
	}
	f.calls = append(f.calls, args[0])
	switch args[0] {
	case "run":
		if f.failOnRun {
			return exec.Output{ExitCode: 1}, &envelope.Error{Code: "tool_failed", Message: "mutmut run: exit 1"}
		}
		return exec.Output{}, nil
	case "junitxml":
		if f.failOnJXML {
			return exec.Output{ExitCode: 1}, &envelope.Error{Code: "tool_failed", Message: "mutmut junitxml: exit 1"}
		}
		data, err := os.ReadFile(f.junitxml)
		if err != nil {
			f.t.Fatal(err)
		}
		return exec.Output{Stdout: string(data)}, nil
	default:
		f.t.Fatalf("unexpected verb %q", args[0])
		return exec.Output{}, nil
	}
}

func TestToolRun(t *testing.T) {
	repo := detect.Repo{Name: "svc", LocalPath: t.TempDir()}
	scope := mutation.Scope{Repo: "svc", Files: []string{"pkg/add.py"}}
	fx := &fakeExecer{t: t, junitxml: "testdata/pass.xml"}

	report, err := Tool{}.Run(context.Background(), repo, scope, fx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 2 || report.Killed != 2 {
		t.Errorf("Run() = %+v", report)
	}
	if len(fx.calls) != 2 || fx.calls[0] != "run" || fx.calls[1] != "junitxml" {
		t.Errorf("calls = %v, want [run junitxml]", fx.calls)
	}
}

func TestToolRunTolerantOfRunExitNonZero(t *testing.T) {
	repo := detect.Repo{Name: "svc", LocalPath: t.TempDir()}
	scope := mutation.Scope{Repo: "svc", Files: []string{"pkg/add.py"}}
	fx := &fakeExecer{t: t, junitxml: "testdata/below_threshold.xml", failOnRun: true}

	report, err := Tool{}.Run(context.Background(), repo, scope, fx, 0)
	if err != nil {
		t.Fatalf("Run() with a nonzero `mutmut run` exit should still parse junitxml, got err: %v", err)
	}
	if report.Total != 3 || len(report.Survivors) != 1 {
		t.Errorf("Run() = %+v", report)
	}
}

func TestToolRunJunitxmlToolFailedPropagates(t *testing.T) {
	repo := detect.Repo{Name: "svc", LocalPath: t.TempDir()}
	scope := mutation.Scope{Repo: "svc", Files: []string{"pkg/add.py"}}
	fx := &fakeExecer{t: t, failOnJXML: true}

	tool := Tool{}
	if _, err := tool.Run(context.Background(), repo, scope, fx, 0); err == nil {
		t.Error("Run() with junitxml failure = nil error, want one (tool_failed still has no report to parse)")
	}
}

func TestToolRunEmptyScope(t *testing.T) {
	fx := &fakeExecer{t: t}
	report, err := Tool{}.Run(context.Background(), detect.Repo{Name: "svc"}, mutation.Scope{}, fx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 0 {
		t.Errorf("Run(empty scope) = %+v, want zero report", report)
	}
	if len(fx.calls) != 0 {
		t.Errorf("calls = %v, want none", fx.calls)
	}
}
