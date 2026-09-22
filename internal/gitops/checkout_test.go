package gitops

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestCheckoutBaseAndBranch(t *testing.T) {
	fe := exectest.New(t).
		On("git", []string{"checkout", "main"}, exec.Output{}).
		On("git", []string{"checkout", "feature/PROJ-1"}, exec.Output{})
	g := New(fe, 0)

	if err := g.CheckoutBase(context.Background(), "/repo", "main"); err != nil {
		t.Fatalf("CheckoutBase: %v", err)
	}
	if err := g.CheckoutBranch(context.Background(), "/repo", "feature/PROJ-1"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}
	if len(fe.Calls) != 2 || fe.Calls[0].Dir != "/repo" || fe.Calls[1].Dir != "/repo" {
		t.Errorf("calls = %+v, want both against /repo", fe.Calls)
	}
}
