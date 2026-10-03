package check_test

import (
	"context"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

// golangciArgs is golangci-lint v2's own argv for a check with no
// config — verified against a real v2.13.2 binary (phase 33).
var golangciArgs = []string{"run", "--output.json.path", "stdout", "--show-stats=false", "--path-mode", "abs", "./..."}

func TestRunLintGolangciLint(t *testing.T) {
	report := `{"Issues":[{"Text":"unused variable","Pos":{"Filename":"/repo/cmd/main.go","Line":10},"FromLinter":"unused"}]}`
	x := exectest.New(t).On("golangci-lint", golangciArgs, exec.Output{Stdout: report})

	runner, ok := check.ForKind("lint")
	if !ok {
		t.Fatal("no lint runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "go.lint", Tool: "golangci-lint", Severity: "must"},
		Repo:  check.Repo{Name: "acme.api", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want 1", findings)
	}
	f := findings[0]
	if f.File != "cmd/main.go" || f.Line != 10 || f.Status != "open" {
		t.Errorf("finding = %+v", f)
	}
	if f.Message != "[unused] unused variable" {
		t.Errorf("message = %q", f.Message)
	}
}

func TestRunLintGolangciLintClean(t *testing.T) {
	x := exectest.New(t).On("golangci-lint", golangciArgs, exec.Output{Stdout: `{"Issues":[]}`})
	runner, _ := check.ForKind("lint")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "go.lint", Tool: "golangci-lint", Severity: "must"},
		Repo:  check.Repo{Name: "acme.api", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestRunLintESLint(t *testing.T) {
	report := `[{"filePath":"src/app.ts","messages":[{"line":5,"message":"no-explicit-any","ruleId":"@typescript-eslint/no-explicit-any"}]}]`
	x := exectest.New(t).On("eslint", []string{"--format", "json", "."}, exec.Output{Stdout: report})
	runner, _ := check.ForKind("lint")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "typescript.lint", Tool: "eslint", Severity: "must"},
		Repo:  check.Repo{Name: "acme.web", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "src/app.ts" || findings[0].Line != 5 {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRunLintRuffCheck(t *testing.T) {
	report := `[{"filename":"app.py","message":"unused import","code":"F401","location":{"row":3}}]`
	x := exectest.New(t).On("ruff", []string{"check", "--output-format", "json", "."}, exec.Output{Stdout: report})
	runner, _ := check.ForKind("lint")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "python.lint", Tool: "ruff", Severity: "should"},
		Repo:  check.Repo{Name: "acme.svc", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "app.py" || findings[0].Line != 3 {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRunLintCheckstyle(t *testing.T) {
	report := `<?xml version="1.0"?>
<checkstyle version="10.0">
  <file name="Main.java">
    <error line="12" severity="error" message="Missing Javadoc"/>
  </file>
</checkstyle>`
	x := exectest.New(t).On("checkstyle", []string{"-f", "xml", "."}, exec.Output{Stdout: report})
	runner, _ := check.ForKind("lint")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "java.lint", Tool: "checkstyle", Severity: "should"},
		Repo:  check.Repo{Name: "acme.core", LocalPath: "/repo"},
	}, x, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 1 || findings[0].File != "Main.java" || findings[0].Line != 12 || findings[0].Message != "Missing Javadoc" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRunLintUnknownTool(t *testing.T) {
	runner, _ := check.ForKind("lint")
	_, err := runner.Run(context.Background(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "x", Tool: "mystery-linter"},
		Repo:  check.Repo{Name: "r", LocalPath: "/repo"},
	}, exectest.New(t), time.Second)
	if err == nil {
		t.Fatal("expected an error for an unregistered tool")
	}
}
