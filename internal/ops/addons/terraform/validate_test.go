package terraform_test

import (
	"context"
	"os"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestValidateMapsTflintFindings(t *testing.T) {
	report, err := os.ReadFile("testdata/tflint.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	x := exectest.New(t)
	x.OnFailWithOutput("tflint", []string{"--format", "json"},
		exec.Output{Stdout: string(report), ExitCode: 2},
		&envelope.Error{Code: "tool_failed", Message: "tflint found issues"},
	)

	a := terraform.New(x, &config.Config{}, "")
	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %v, want 2", findings)
	}
	if findings[0].Tool != "tflint" || findings[0].Severity != "warning" || findings[0].File != "main.tf" || findings[0].Line != 17 {
		t.Errorf("findings[0] = %+v, unexpected", findings[0])
	}
	if findings[1].Severity != "error" {
		t.Errorf("findings[1].Severity = %q, want error", findings[1].Severity)
	}
}

func TestValidatePropagatesRealFailure(t *testing.T) {
	x := exectest.New(t)
	x.OnFail("tflint", []string{"--format", "json"}, &envelope.Error{Code: "internal", Message: "spawn failed"})

	a := terraform.New(x, &config.Config{}, "")
	if _, err := a.Validate(context.Background(), &ops.Rendered{Dir: t.TempDir()}); err == nil {
		t.Fatal("Validate() err = nil, want the spawn failure propagated")
	}
}
