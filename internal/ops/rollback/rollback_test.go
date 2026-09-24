package rollback

import (
	"context"
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/testdata/fixtureaddon"
)

func TestDeriveOneRevertPerRepositoryRegardlessOfTargets(t *testing.T) {
	res, err := Derive(context.Background(), []Repository{{Name: "acme.infra"}, {Name: "acme.app"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repositories) != 2 {
		t.Fatalf("len(Repositories) = %d, want 2", len(res.Repositories))
	}
	if res.Repositories[0].Revert == "" || res.Repositories[1].Revert == "" {
		t.Error("Revert text must not be empty")
	}
	if len(res.Targets) != 0 || len(res.NonRevertible) != 0 || len(res.WatchList) != 0 {
		t.Errorf("with no targets: Targets=%v NonRevertible=%v WatchList=%v, want all empty", res.Targets, res.NonRevertible, res.WatchList)
	}
}

func TestDeriveSkipsAddonWithoutRollbackCapability(t *testing.T) {
	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.Caps.Rollback = false
	addon.RollbackInfo = &ops.RollbackInfo{Steps: []string{"should not appear"}}

	target := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/prod"}
	res, err := Derive(context.Background(), nil, []TargetInput{{Target: target, Addon: addon}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Targets) != 0 {
		t.Errorf("Targets = %v, want empty (Rollback capability false)", res.Targets)
	}
}

func TestDeriveSkipsUnregisteredAddon(t *testing.T) {
	target := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/prod"}
	res, err := Derive(context.Background(), nil, []TargetInput{{Target: target, Addon: nil}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Targets) != 0 {
		t.Errorf("Targets = %v, want empty (nil addon skipped)", res.Targets)
	}
}

func TestDeriveNonRevertibleDedupedAcrossTargets(t *testing.T) {
	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.RollbackInfo = &ops.RollbackInfo{NonRevertible: []string{"aws_s3_bucket.data", "aws_iam_role.x"}}

	t1 := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/a"}
	t2 := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/b"}
	res, err := Derive(context.Background(), nil, []TargetInput{
		{Target: t1, Addon: addon},
		{Target: t2, Addon: addon},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"aws_iam_role.x", "aws_s3_bucket.data"}
	if !equalStrings(res.NonRevertible, want) {
		t.Errorf("NonRevertible = %v, want %v (deduped and sorted)", res.NonRevertible, want)
	}
}

func TestDeriveWatchListExcludesNoop(t *testing.T) {
	addon := fixtureaddon.New("fixture", "infra/prod")
	target := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/prod"}
	cs := ops.Changeset{Actions: []ops.Action{
		{Address: "aws_instance.web", Action: ops.ActionUpdate},
		{Address: "aws_instance.noop", Action: ops.ActionNoop},
	}}
	res, err := Derive(context.Background(), nil, []TargetInput{{Target: target, Addon: addon, Changeset: cs}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"aws_instance.web"}
	if !equalStrings(res.WatchList, want) {
		t.Errorf("WatchList = %v, want %v", res.WatchList, want)
	}
}

func TestDeriveTargetsSortedByTargetKey(t *testing.T) {
	addon := fixtureaddon.New("fixture", "infra/prod")
	tb := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/b"}
	ta := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/a"}
	res, err := Derive(context.Background(), nil, []TargetInput{
		{Target: tb, Addon: addon},
		{Target: ta, Addon: addon},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Targets) != 2 || res.Targets[0].Target != ops.TargetKey(ta) {
		t.Errorf("Targets = %+v, want sorted with %s first", res.Targets, ops.TargetKey(ta))
	}
}

func TestDerivePropagatesAddonError(t *testing.T) {
	addon := &erroringAddon{Addon: *fixtureaddon.New("fixture", "infra/prod")}
	target := ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/prod"}
	_, err := Derive(context.Background(), nil, []TargetInput{{Target: target, Addon: addon}})
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

type erroringAddon struct {
	fixtureaddon.Addon
}

func (a *erroringAddon) Rollback(_ context.Context, _ ops.Target, _ ops.Env, _ *ops.Changeset) (*ops.RollbackInfo, error) {
	return nil, errors.New("boom")
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
