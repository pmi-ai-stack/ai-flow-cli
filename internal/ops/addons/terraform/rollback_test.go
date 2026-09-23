package terraform_test

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
)

func TestRollbackSplitsNonRevertibleFromRevertible(t *testing.T) {
	cs := &ops.Changeset{Actions: []ops.Action{
		{Address: "aws_s3_bucket.old", Action: ops.ActionDelete, KindClass: ops.KindData},
		{Address: "aws_instance.web", Action: ops.ActionUpdate, KindClass: ops.KindWorkload},
		{Address: "aws_iam_role.web", Action: ops.ActionReplace, KindClass: ops.KindIAM},
		{Address: "aws_instance.noop", Action: ops.ActionNoop, KindClass: ops.KindWorkload},
	}}

	a := terraform.New(nil, &config.Config{}, "")
	info, err := a.Rollback(context.Background(), ops.Target{Path: "infra/prod"}, ops.Env{}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.NonRevertible) != 2 || info.NonRevertible[0] != "aws_iam_role.web" || info.NonRevertible[1] != "aws_s3_bucket.old" {
		t.Fatalf("NonRevertible = %v, want [aws_iam_role.web aws_s3_bucket.old] (sorted by address)", info.NonRevertible)
	}
	if len(info.Steps) != 3 { // one general instruction + one per non-revertible address
		t.Fatalf("Steps = %v, want 3", info.Steps)
	}
}

func TestRollbackNeverCallsExec(t *testing.T) {
	// x is nil: Rollback must never dereference it — proves the "no
	// subprocess at all" contract structurally, not just by absence of
	// registered calls on a Scripted double.
	a := terraform.New(nil, &config.Config{}, "")
	if _, err := a.Rollback(context.Background(), ops.Target{}, ops.Env{}, &ops.Changeset{}); err != nil {
		t.Fatal(err)
	}
}
