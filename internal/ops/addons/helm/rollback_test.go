package helm

import (
	"context"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

const helmHistoryOutput = "REVISION\tUPDATED\tSTATUS\tCHART\tAPP VERSION\tDESCRIPTION\n" +
	"1       \tMon Jan  1 00:00:00 2024\tsuperseded\tdemo-0.1.0\t1.0        \tInstall complete\n" +
	"2       \tTue Jan  2 00:00:00 2024\tdeployed  \tdemo-0.1.1\t1.0        \tUpgrade complete\n"

func TestRollbackNamesCommandWithPreviousRevisionOnceForTheWholeRelease(t *testing.T) {
	x := exectest.New(t)
	x.On("helm", []string{"history", "demo", "--kube-context", "prod-ctx"}, exec.Output{Stdout: helmHistoryOutput})

	a := New(x, boundConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionUpdate, Address: "default/Deployment/web", Kind: "Deployment", KindClass: ops.KindWorkload},
		{Action: ops.ActionUpdate, Address: "default/ConfigMap/cfg", Kind: "ConfigMap", KindClass: ops.KindOther},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{Path: "testdata/chart"}, ops.Env{Name: "prod", Tier: "prod"}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Calls) != 1 {
		t.Fatalf("Rollback made %d \"helm history\" calls, want exactly one regardless of action count", len(x.Calls))
	}
	found := false
	for _, s := range info.Steps {
		if strings.Contains(s, "helm rollback demo --kube-context prod-ctx 1") && strings.Contains(s, "revision before") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Steps = %v, want one naming the rollback command with the previous revision (1)", info.Steps)
	}
}

func TestRollbackTolerantOfHistoryFailure(t *testing.T) {
	x := exectest.New(t)
	x.OnFail("helm", []string{"history", "demo", "--kube-context", "prod-ctx"}, &envelope.Error{Code: "tool_failed", Message: "boom"})

	a := New(x, boundConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionUpdate, Address: "default/Deployment/web", Kind: "Deployment", KindClass: ops.KindWorkload},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{Path: "testdata/chart"}, ops.Env{Name: "prod", Tier: "prod"}, cs)
	if err != nil {
		t.Fatalf("Rollback returned an error on a history failure, want a tolerant step: %v", err)
	}
	found := false
	for _, s := range info.Steps {
		if strings.Contains(s, "helm rollback demo") && strings.Contains(s, "could not be read") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Steps = %v, want a tolerant step naming the rollback command with no revision", info.Steps)
	}
}

func TestRollbackFlagsNonRevertibleDeleteOnProtectedClasses(t *testing.T) {
	x := exectest.New(t)
	a := New(x, boundConfig())
	cs := &ops.Changeset{Actions: []ops.Action{
		{Action: ops.ActionDelete, Address: "default/Secret/creds", Kind: "Secret", KindClass: ops.KindSecret},
		{Action: ops.ActionDelete, Address: "default/ConfigMap/cfg", Kind: "ConfigMap", KindClass: ops.KindOther},
	}}
	info, err := a.Rollback(context.Background(), ops.Target{Path: "testdata/chart"}, ops.Env{Name: "prod", Tier: "prod"}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.NonRevertible) != 1 || info.NonRevertible[0] != "default/Secret/creds" {
		t.Fatalf("NonRevertible = %v, want exactly [default/Secret/creds]", info.NonRevertible)
	}
	if len(x.Calls) != 0 {
		t.Fatalf("Rollback made %d calls, want zero — no update/replace action needs a rollback step", len(x.Calls))
	}
}
