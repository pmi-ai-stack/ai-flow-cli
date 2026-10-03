package check_test

import (
	"context"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

// failingExecer answers every call with out and err.
type failingExecer struct {
	out exec.Output
	err error
}

func (f failingExecer) Run(context.Context, string, []string, exec.RunOpts) (exec.Output, error) {
	return f.out, f.err
}

// TestJSONToolFailingWithoutReportIsAnError pins the bug the phase-33
// e2e run found: conftest panicking without $HOME (exit 2, nothing on
// stdout) used to read as a clean pass. A non-zero exit with no report
// is an error; with a report it is the tool's own "found something".
func TestJSONToolFailingWithoutReportIsAnError(t *testing.T) {
	crashed := &envelope.Error{Code: "tool_failed", Message: "conftest test: exit status 2: panic: path in XDG_DATA_HOME is relative"}

	policy := standards.EffectiveCheck{
		ID: "infra.terraform.policy", Kind: "policy", Tool: "conftest", Severity: "must",
		Config: "policy", Pack: packWithConfigDir("policy", map[string]string{"naming.rego": "package main\n"}),
	}
	lint := standards.EffectiveCheck{ID: "go.lint", Kind: "lint", Tool: "golangci-lint", Severity: "must"}
	target := check.Target{Key: "r:.", Addon: "terraform", Repo: "r", SourceDir: t.TempDir()}
	repo := check.Repo{Name: "r", LocalPath: t.TempDir()}

	for _, tc := range []struct {
		name string
		c    standards.EffectiveCheck
		in   check.RunInput
	}{
		{"policy", policy, check.RunInput{Check: policy, Target: target}},
		{"lint", lint, check.RunInput{Check: lint, Repo: repo}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, _ := check.ForKind(tc.c.Kind)

			_, err := runner.Run(context.Background(), tc.in, failingExecer{err: crashed}, time.Second)
			if err == nil {
				t.Fatal("tool_failed with empty stdout = nil error, want the failure")
			}

			_, err = runner.Run(context.Background(), tc.in, failingExecer{err: exec.NotFound(tc.c.Tool)}, time.Second)
			if _, ok := exec.IsNotFound(err); !ok {
				t.Errorf("missing tool: err = %v, want a not-found error the caller can report", err)
			}
		})
	}

	// The report-carrying failure is still a result, not an error.
	runner, _ := check.ForKind("policy")
	findings, err := runner.Run(context.Background(), check.RunInput{Check: policy, Target: target},
		failingExecer{out: exec.Output{Stdout: `[{"filename":"main.tf","failures":[{"msg":"bad"}]}]`}, err: crashed}, time.Second)
	if err != nil || len(findings) != 1 {
		t.Errorf("tool_failed with a report = (%v, %v), want one finding", findings, err)
	}
}
