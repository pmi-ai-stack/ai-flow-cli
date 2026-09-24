package kubectl

import (
	"context"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

const rolloutHistoryOutput = "deployment.apps/web\nREVISION  CHANGE-CAUSE\n1         <none>\n2         <none>\n"

func TestRollbackNamesUndoWithCurrentRevisionForRolloutCapableUpdate(t *testing.T) {
	x := exectest.New(t)
	x.On("kubectl", []string{"rollout", "history", "Deployment/web", "-n", "default", "--context", "prod-ctx"}, exec.Output{Stdout: rolloutHistoryOutput})

	a := New(x, boundConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionUpdate, Address: "default/Deployment/web", Kind: "Deployment", KindClass: ops.KindWorkload},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{Path: "manifests"}, ops.Env{Name: "prod", Tier: "prod"}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.NonRevertible) != 0 {
		t.Errorf("NonRevertible = %v, want none for a Deployment update", info.NonRevertible)
	}
	found := false
	for _, s := range info.Steps {
		if strings.Contains(s, "kubectl rollout undo Deployment/web -n default --context prod-ctx") && strings.Contains(s, "revision is 2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Steps = %v, want one naming the undo command with the current revision", info.Steps)
	}
}

func TestRollbackTolerantOfRolloutHistoryFailure(t *testing.T) {
	x := exectest.New(t)
	x.OnFail("kubectl", []string{"rollout", "history", "StatefulSet/bare", "-n", "default", "--context", "prod-ctx"}, &envelope.Error{Code: "tool_failed", Message: "boom"})

	a := New(x, boundConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionUpdate, Address: "default/StatefulSet/bare", Kind: "StatefulSet", KindClass: ops.KindWorkload},
	}}

	info, err := a.Rollback(context.Background(), ops.Target{}, ops.Env{Name: "prod", Tier: "prod"}, cs)
	if err != nil {
		t.Fatalf("Rollback returned an error on a rollout-history failure, want a tolerant step: %v", err)
	}
	found := false
	for _, s := range info.Steps {
		if strings.Contains(s, "kubectl rollout undo StatefulSet/bare") && strings.Contains(s, "could not be read") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Steps = %v, want a tolerant step naming the undo command with no revision", info.Steps)
	}
}

func TestRollbackFlagsNonRevertibleDeleteOnProtectedClasses(t *testing.T) {
	a := New(exectest.New(t), boundConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionDelete, Address: "default/Secret/creds", Kind: "Secret", KindClass: ops.KindSecret},
		{Action: ops.ActionDelete, Address: "default/ConfigMap/cfg", Kind: "ConfigMap", KindClass: ops.KindOther},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{}, ops.Env{Name: "prod", Tier: "prod"}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.NonRevertible) != 1 || info.NonRevertible[0] != "default/Secret/creds" {
		t.Fatalf("NonRevertible = %v, want exactly [default/Secret/creds]", info.NonRevertible)
	}
}

func TestRollbackNoLiveBindingStillProducesAStep(t *testing.T) {
	a := New(exectest.New(t), &config.Config{})
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionUpdate, Address: "default/Deployment/web", Kind: "Deployment", KindClass: ops.KindWorkload},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{}, ops.Env{}, cs)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range info.Steps {
		if strings.Contains(s, "no live context bound") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Steps = %v, want a step noting no live context is bound", info.Steps)
	}
}
