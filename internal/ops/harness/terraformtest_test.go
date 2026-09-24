package harness_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
)

// TestTerraformTestParsesRealCapturedOutput parses this session's own
// real `terraform test -json` output (terraform v1.16.4, hand-run
// against a throwaway fixture module — see the phase-24 plan's §0),
// not a documented-but-unconfirmed shape.
func TestTerraformTestParsesRealCapturedOutput(t *testing.T) {
	data, err := os.ReadFile("testdata/terraform_test_pass_fail.jsonl")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := exectest.New(t).OnFailWithOutput(
		"terraform", []string{"test", "-json", "-no-color", "-filter=main.tftest.hcl"},
		exec.Output{Stdout: string(data), ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "terraform test: exit status 1"},
	)

	res, err := harness.Run(context.Background(), s, harness.RunInput{
		Tool: "terraform-test", Dir: "/tmp/module", Filter: "main.tftest.hcl", Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("Run: %v, want a failed run block to parse cleanly, not error", err)
	}
	if res.Total != 2 || res.Passed != 1 || res.Failed != 1 || res.Skipped != 0 {
		t.Fatalf("Run = %+v, want total=2 passed=1 failed=1", res)
	}
	if len(res.Failures) != 1 || res.Failures[0] != "fails_on_purpose: doubled should be 999 (intentionally wrong)" {
		t.Errorf("Failures = %v", res.Failures)
	}
	if len(res.Raw) == 0 {
		t.Error("Raw is empty, want the captured stdout")
	}
}

func TestTerraformTestZeroRunBlocksIsNotAFailure(t *testing.T) {
	s := exectest.New(t).On(
		"terraform", []string{"test", "-json", "-no-color", "-filter=nope.tftest.hcl"},
		exec.Output{Stdout: `{"type":"test_abstract","test_abstract":{}}
{"type":"test_summary","test_summary":{"status":"pass","passed":0,"failed":0,"errored":0,"skipped":0}}
`},
	)
	res, err := harness.Run(context.Background(), s, harness.RunInput{Tool: "terraform-test", Dir: "/tmp/module", Filter: "nope.tftest.hcl"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 0 || res.Passed != 0 || res.Failed != 0 {
		t.Errorf("Run = %+v, want all-zero", res)
	}
}

func TestTerraformTestExecNotAllowedPropagates(t *testing.T) {
	s := exectest.New(t).OnFail("terraform", []string{"test", "-json", "-no-color", "-filter=x.tftest.hcl"},
		exec.NotAllowed([]string{"terraform", "test", "-json", "-no-color", "-filter=x.tftest.hcl"}))
	_, err := harness.Run(context.Background(), s, harness.RunInput{Tool: "terraform-test", Dir: "/tmp/module", Filter: "x.tftest.hcl"})
	if err == nil {
		t.Fatal("Run: want exec_not_allowed to propagate, got nil error")
	}
}

func TestTerraformTestErroredFoldsIntoFailed(t *testing.T) {
	s := exectest.New(t).OnFailWithOutput(
		"terraform", []string{"test", "-json", "-no-color", "-filter=err.tftest.hcl"},
		exec.Output{Stdout: `{"type":"test_summary","test_summary":{"status":"error","passed":0,"failed":0,"errored":1,"skipped":0}}
`},
		&envelope.Error{Code: "tool_failed", Message: "terraform test: exit status 1"},
	)
	res, err := harness.Run(context.Background(), s, harness.RunInput{Tool: "terraform-test", Dir: "/tmp/module", Filter: "err.tftest.hcl", Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 1 || res.Failed != 1 {
		t.Errorf("Run = %+v, want errored folded into failed (total=1 failed=1)", res)
	}
}
