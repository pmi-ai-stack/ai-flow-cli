package check_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

func toolFailed(out exec.Output) (exec.Output, error) {
	return out, &envelope.Error{Code: "tool_failed", Message: "tool exited non-zero"}
}

func TestRunFormatGofmtCheck(t *testing.T) {
	x := exectest.New(t).On("gofmt", []string{"-l", "."}, exec.Output{Stdout: "main.go\ncmd/foo.go\n"})
	runner, ok := check.ForKind("format")
	if !ok {
		t.Fatal("no format runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "go.format", Tool: "gofmt", Severity: "must"},
		Repo:  check.Repo{Name: "acme.api", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v, want 2", findings)
	}
	for _, f := range findings {
		if f.Status != "open" {
			t.Errorf("finding %+v: status = %q, want open", f, f.Status)
		}
	}
	if findings[0].File != "main.go" || findings[1].File != "cmd/foo.go" {
		t.Errorf("unexpected file names: %+v", findings)
	}
}

func TestRunFormatGofmtFix(t *testing.T) {
	x := exectest.New(t).
		On("gofmt", []string{"-l", "."}, exec.Output{Stdout: "main.go\n"}).
		On("gofmt", []string{"-w", "."}, exec.Output{})
	runner, _ := check.ForKind("format")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "go.format", Tool: "gofmt", Severity: "must"},
		Repo:  check.Repo{Name: "acme.api", LocalPath: "/repo"},
		Fix:   true,
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].Status != "fixed" || !findings[0].AutofixApplied {
		t.Fatalf("findings = %#v, want one fixed/autofix_applied finding", findings)
	}
}

func TestRunFormatGofmtCleanNoFix(t *testing.T) {
	x := exectest.New(t).On("gofmt", []string{"-l", "."}, exec.Output{})
	runner, _ := check.ForKind("format")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "go.format", Tool: "gofmt", Severity: "must"},
		Repo:  check.Repo{Name: "acme.api", LocalPath: "/repo"},
		Fix:   true,
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none (nothing to fix, gofmt -w never called)", findings)
	}
	if x.CalledWithPrefix("gofmt", "-w") {
		t.Error("gofmt -w was called even though nothing needed fixing")
	}
}

func TestRunFormatPrettierCheck(t *testing.T) {
	x := exectest.New(t)
	out, e := toolFailed(exec.Output{
		Stdout: "Checking formatting...\nsrc/App.tsx\n[warn] src/index.ts\nCode style issues found in 2 files.\n",
	})
	x.OnFailWithOutput("prettier", []string{"--check", "."}, out, e)

	runner, _ := check.ForKind("format")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "ts.format", Tool: "prettier", Severity: "must"},
		Repo:  check.Repo{Name: "acme.web", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v, want 2", findings)
	}
	got := []string{findings[0].File, findings[1].File}
	want := []string{"src/App.tsx", "src/index.ts"}
	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestRunFormatRuffFormatCheckAndFix(t *testing.T) {
	scratch := t.TempDir()
	configPath := filepath.Join(scratch, "config-python.format.toml")

	x := exectest.New(t)
	checkOut, checkErr := toolFailed(exec.Output{Stdout: "Would reformat: app.py\n"})
	x.OnFailWithOutput("ruff", []string{"format", "--check", "--config", configPath, "."}, checkOut, checkErr)
	x.On("ruff", []string{"format", "--config", configPath, "."}, exec.Output{})

	c := standards.EffectiveCheck{
		ID: "python.format", Tool: "ruff", Severity: "must", Config: "ruff.toml",
		Pack: packWithConfig("ruff.toml", "line-length = 100\n"),
	}

	runner, _ := check.ForKind("format")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: c, Repo: check.Repo{Name: "acme.svc", LocalPath: "/repo"}, Fix: true, Scratch: scratch,
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "app.py" || findings[0].Status != "fixed" {
		t.Fatalf("findings = %#v", findings)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Errorf("materialized config at %s should have been cleaned up after the run", configPath)
	}
}
