package kustomize

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

const kubeconformReportJSON = `{
  "resources": [
    {"filename": "rendered.yaml", "kind": "Deployment", "name": "web", "version": "apps/v1", "status": "valid", "msg": ""},
    {"filename": "rendered.yaml", "kind": "Secret", "name": "web-credentials", "version": "v1", "status": "invalid", "msg": "missing required field"},
    {"filename": "rendered.yaml", "kind": "", "name": "", "version": "", "status": "error", "msg": "could not parse"},
    {"filename": "rendered.yaml", "kind": "Widget", "name": "w", "version": "example.com/v1", "status": "skipped", "msg": "no schema"}
  ]
}`

func TestValidateMapsInvalidAndErrorStatusesToFindings(t *testing.T) {
	x := exectest.New(t)
	x.On("kubeconform", []string{"-output", "json", "-summary"}, exec.Output{Stdout: kubeconformReportJSON})

	a := New(x, &config.Config{})
	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: "/rendered"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("Validate = %v, want two findings (invalid + error; valid/skipped produce none)", findings)
	}
	if findings[0].Severity != "error" {
		t.Errorf("findings[0].Severity = %q, want error", findings[0].Severity)
	}
	if findings[1].Severity != "warning" {
		t.Errorf("findings[1].Severity = %q, want warning", findings[1].Severity)
	}
}

func TestValidateToleratesNonZeroExit(t *testing.T) {
	x := exectest.New(t)
	x.OnFailWithOutput("kubeconform", []string{"-output", "json", "-summary"},
		exec.Output{Stdout: kubeconformReportJSON, ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "kubeconform found issues"},
	)

	a := New(x, &config.Config{})
	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: "/rendered"})
	if err != nil {
		t.Fatalf("Validate returned an error on a findings-shaped non-zero exit: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("Validate = %v, want two findings parsed despite the non-zero exit", findings)
	}
}
