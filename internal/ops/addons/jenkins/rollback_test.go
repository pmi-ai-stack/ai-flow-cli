package jenkins_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
)

func TestRollbackIsTextOnlyNeverNonRevertible(t *testing.T) {
	a := jenkins.New(nil, devConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionUpdate, Address: "Jenkinsfile", KindClass: ops.KindPipeline},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{Path: "Jenkinsfile"}, ops.Env{}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.NonRevertible) != 0 {
		t.Errorf("NonRevertible = %v, want none", info.NonRevertible)
	}
	if len(info.Steps) != 1 || !strings.Contains(info.Steps[0], "git revert") || !strings.Contains(info.Steps[0], "Jenkinsfile") {
		t.Errorf("Steps = %v, want one git-revert step naming Jenkinsfile", info.Steps)
	}
}
