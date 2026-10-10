package helm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// realHelmLintOutput is verified against a real "helm lint" v3.16.2
// binary — see .ai/phase-26/plan.md §0.1 (the "break it" probe: a
// Chart.yaml missing its required "version" field).
const realHelmLintOutput = "==> Linting testdata/chart\n" +
	"[ERROR] Chart.yaml: version is required\n" +
	"[INFO] Chart.yaml: icon is recommended\n" +
	"[ERROR] templates/: validation: chart.metadata.version is required\n\n" +
	"Error: 1 chart(s) linted, 1 chart(s) failed\n"

func TestValidateParsesRealHelmLintOutputShape(t *testing.T) {
	renderedDir := t.TempDir()
	chartDir := filepath.Join(renderedDir, "chart")
	mustMkdirAll(t, chartDir)

	x := exectest.New(t)
	x.OnFailWithOutput("helm", []string{"lint", chartDir}, exec.Output{Stdout: realHelmLintOutput, ExitCode: 1}, &envelope.Error{Code: "tool_failed", Message: "1 chart(s) failed"})
	x.On("kubeconform", []string{"-output", "json", "-summary", "rendered.yaml"}, exec.Output{Stdout: `{"resources":[]}`})

	a := New(x, &config.Config{})
	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: renderedDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("Validate = %v, want two findings (two [ERROR] lines; the [INFO] line produces none)", findings)
	}
	for _, f := range findings {
		if f.Severity != "error" || f.Tool != "helm lint" {
			t.Errorf("finding %+v, want severity error, tool \"helm lint\"", f)
		}
	}
}

func TestValidateConcatenatesHelmLintAndKubeconformFindings(t *testing.T) {
	renderedDir := t.TempDir()
	chartDir := filepath.Join(renderedDir, "chart")
	mustMkdirAll(t, chartDir)

	x := exectest.New(t)
	x.On("helm", []string{"lint", chartDir}, exec.Output{Stdout: "==> Linting " + chartDir + "\n\n1 chart(s) linted, 0 chart(s) failed\n"})
	x.On("kubeconform", []string{"-output", "json", "-summary", "rendered.yaml"}, exec.Output{Stdout: `{"resources":[{"filename":"rendered.yaml","status":"statusInvalid","msg":"bad"}]}`})

	a := New(x, &config.Config{})
	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: renderedDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Tool != "kubeconform" {
		t.Fatalf("Validate = %v, want exactly one kubeconform finding (a clean helm lint contributes none)", findings)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}
