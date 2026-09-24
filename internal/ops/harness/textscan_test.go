package harness_test

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
)

func TestKuttlRecognizesPassFailLines(t *testing.T) {
	s := exectest.New(t).OnFailWithOutput(
		"kuttl", []string{"test", "/render/suite", "--context", "kind-proj-1"},
		exec.Output{Stdout: "=== RUN   kuttl/harness/case-1\n--- PASS: kuttl/harness/case-1 (1.2s)\n--- FAIL: kuttl/harness/case-2 (0.5s)\n", ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "kuttl test: exit status 1"},
	)
	res, err := harness.Run(context.Background(), s, harness.RunInput{
		Tool: "kuttl", Dir: "/render/suite", KubeContext: "kind-proj-1", Kubeconfig: "/home/cache/kind/proj-1.kubeconfig",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 2 || res.Passed != 1 || res.Failed != 1 {
		t.Fatalf("Run = %+v, want total=2 passed=1 failed=1", res)
	}
	if len(res.Failures) != 1 || res.Failures[0] != "kuttl/harness/case-2 (0.5s)" {
		t.Errorf("Failures = %v", res.Failures)
	}
	if len(s.Calls) != 1 || s.Calls[0].Dir != "/render/suite" {
		t.Errorf("Calls = %+v, want one call in /render/suite", s.Calls)
	}
}

func TestKuttlPassesKubeconfigEnv(t *testing.T) {
	s := exectest.New(t).On("kuttl", []string{"test", "/render/suite", "--context", "kind-proj-1"}, exec.Output{Stdout: "--- PASS: a\n"})
	if _, err := harness.Run(context.Background(), s, harness.RunInput{
		Tool: "kuttl", Dir: "/render/suite", KubeContext: "kind-proj-1", Kubeconfig: "/home/cache/kind/proj-1.kubeconfig",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestUnrecognizedOutputFallsBackToExitCode proves the honest fallback
// (plan design decision 6): when kuttl/chainsaw's own output carries
// none of the recognized PASS/FAIL/SKIP lines, the harness still
// reports a single, exit-code-derived verdict rather than a false
// 0/0/0.
func TestUnrecognizedOutputFallsBackToExitCode(t *testing.T) {
	failing := exectest.New(t).OnFailWithOutput(
		"chainsaw", []string{"test", "/render/suite", "--context", "kind-proj-1"},
		exec.Output{Stdout: "some unrecognized tool output\n", ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "chainsaw test: exit status 1"},
	)
	res, err := harness.Run(context.Background(), failing, harness.RunInput{Tool: "chainsaw", Dir: "/render/suite", KubeContext: "kind-proj-1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 1 || res.Passed != 0 || res.Failed != 1 {
		t.Fatalf("Run = %+v, want a single failed verdict from the exit code", res)
	}

	passing := exectest.New(t).On("chainsaw", []string{"test", "/render/suite", "--context", "kind-proj-1"}, exec.Output{Stdout: "some unrecognized tool output\n"})
	res, err = harness.Run(context.Background(), passing, harness.RunInput{Tool: "chainsaw", Dir: "/render/suite", KubeContext: "kind-proj-1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 1 || res.Passed != 1 || res.Failed != 0 {
		t.Fatalf("Run = %+v, want a single passed verdict from the exit code", res)
	}
}

func TestChainsawExecNotAllowedPropagates(t *testing.T) {
	s := exectest.New(t).OnFail("chainsaw", []string{"test", "/render/suite", "--context", "kind-proj-1"},
		exec.NotAllowed([]string{"chainsaw", "test", "/render/suite", "--context", "kind-proj-1"}))
	_, err := harness.Run(context.Background(), s, harness.RunInput{Tool: "chainsaw", Dir: "/render/suite", KubeContext: "kind-proj-1"})
	if err == nil {
		t.Fatal("Run: want exec_not_allowed to propagate")
	}
}
