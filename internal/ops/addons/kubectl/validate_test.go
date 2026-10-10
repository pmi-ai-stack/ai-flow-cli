package kubectl

import (
	"context"
	"os"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// recordedKubeconformReport is real kubeconform v0.8.0 output
// (`-output json -summary .` over a directory holding a valid
// ConfigMap, a Deployment with a string replicas and no selector, and
// a CRD-kind resource with no schema), recorded in phase 58 — the
// first run against a real binary. Only invalid and error resources
// are listed; their statuses are "statusInvalid" and "statusError".
func recordedKubeconformReport(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/kubeconform-report.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// kubeconformArgs is Validate's argv: the rendering directory is the
// path argument; with none, kubeconform reads stdin.
var kubeconformArgs = []string{"-output", "json", "-summary", "."}

func TestValidateMapsInvalidAndErrorStatusesToFindings(t *testing.T) {
	x := exectest.New(t)
	x.On("kubeconform", kubeconformArgs, exec.Output{Stdout: recordedKubeconformReport(t)})

	a := New(x, &config.Config{})
	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: "/rendered"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("Validate = %v, want two findings (one statusError, one statusInvalid)", findings)
	}
	if findings[0].Severity != "warning" || findings[0].File != "crd.yaml" {
		t.Errorf("findings[0] = %+v, want severity warning (statusError), file crd.yaml", findings[0])
	}
	if findings[1].Severity != "error" || findings[1].File != "bad.yaml" {
		t.Errorf("findings[1] = %+v, want severity error (statusInvalid), file bad.yaml", findings[1])
	}
	for _, f := range findings {
		if f.Tool != "kubeconform" {
			t.Errorf("finding %+v has Tool %q, want kubeconform", f, f.Tool)
		}
	}
}

func TestValidateToleratesNonZeroExit(t *testing.T) {
	x := exectest.New(t)
	x.OnFailWithOutput("kubeconform", kubeconformArgs,
		exec.Output{Stdout: recordedKubeconformReport(t), ExitCode: 1},
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
