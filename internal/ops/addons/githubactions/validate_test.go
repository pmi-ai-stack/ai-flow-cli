package githubactions_test

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestValidateCleanWorkflowNoFindings(t *testing.T) {
	x := exectest.New(t)
	x.On("actionlint", []string{"-format", "{{json .}}", githubactions.RenderedFile}, exec.Output{Stdout: "[]"})
	a := githubactions.New(x, devConfig())

	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestValidateReportsActionlintFindings(t *testing.T) {
	report := `[{"message":"unexpected end of input","filepath":"workflow.yml","line":9,"column":26,"kind":"expression"}]`
	x := exectest.New(t)
	x.OnFailWithOutput("actionlint", []string{"-format", "{{json .}}", githubactions.RenderedFile},
		exec.Output{Stdout: report, ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "actionlint found issues"},
	)
	a := githubactions.New(x, devConfig())

	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: "."})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want exactly one", findings)
	}
	if findings[0].Severity != "error" || findings[0].Tool != "actionlint" || findings[0].Line != 9 {
		t.Errorf("findings[0] = %+v, want severity error, tool actionlint, line 9", findings[0])
	}
}
