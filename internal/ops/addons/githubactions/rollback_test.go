package githubactions_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
)

func TestRollbackIsTextOnlyNeverNonRevertible(t *testing.T) {
	a := githubactions.New(nil, devConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionUpdate, Address: ".github/workflows/ci.yml", KindClass: ops.KindPipeline},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{Path: ".github/workflows/ci.yml"}, ops.Env{}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.NonRevertible) != 0 {
		t.Errorf("NonRevertible = %v, want none (kind_class pipeline is never non-revertible)", info.NonRevertible)
	}
	if len(info.Steps) != 1 || !strings.Contains(info.Steps[0], "git revert") || !strings.Contains(info.Steps[0], ".github/workflows/ci.yml") {
		t.Errorf("Steps = %v, want one git-revert step naming the target path", info.Steps)
	}
}
