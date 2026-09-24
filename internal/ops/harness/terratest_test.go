package harness_test

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
)

const terratestJSONL = `{"Action":"run","Test":"TestSandboxNetwork"}
{"Action":"output","Test":"TestSandboxNetwork","Output":"=== RUN   TestSandboxNetwork\n"}
{"Action":"pass","Test":"TestSandboxNetwork"}
{"Action":"run","Test":"TestSandboxNetworkFails"}
{"Action":"output","Test":"TestSandboxNetworkFails","Output":"    net_test.go:12: expected 2 subnets, got 1\n"}
{"Action":"fail","Test":"TestSandboxNetworkFails"}
`

func TestTerratestParsesGoTestJSON(t *testing.T) {
	s := exectest.New(t).OnFailWithOutput(
		"go", []string{"test", "-json", "./..."},
		exec.Output{Stdout: terratestJSONL, ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "go test: exit status 1"},
	)
	res, err := harness.Run(context.Background(), s, harness.RunInput{Tool: "terratest", Dir: "/tmp/module/test"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 2 || res.Passed != 1 || res.Failed != 1 {
		t.Fatalf("Run = %+v, want total=2 passed=1 failed=1", res)
	}
	if len(res.Failures) != 1 || res.Failures[0] != "TestSandboxNetworkFails: net_test.go:12: expected 2 subnets, got 1" {
		t.Errorf("Failures = %v", res.Failures)
	}
}

func TestTerratestAllPass(t *testing.T) {
	s := exectest.New(t).On("go", []string{"test", "-json", "./..."}, exec.Output{Stdout: `{"Action":"run","Test":"TestOK"}
{"Action":"pass","Test":"TestOK"}
`})
	res, err := harness.Run(context.Background(), s, harness.RunInput{Tool: "terratest", Dir: "/tmp/module/test"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 1 || res.Passed != 1 || res.Failed != 0 {
		t.Errorf("Run = %+v, want total=1 passed=1", res)
	}
}
