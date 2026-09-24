package check_test

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

func TestTargetInputDirNoTargetIsError(t *testing.T) {
	runner, _ := check.ForKind("policy")
	_, err := runner.Run(t.Context(), check.RunInput{
		Check: standards.EffectiveCheck{ID: "infra.kubernetes.policy", Tool: "conftest", Severity: "must", Config: "policy"},
	}, nil, 0)
	if err == nil {
		t.Fatal("Run: want an error for a check.Target-less RunInput, got nil")
	}
}
