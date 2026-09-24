package risk

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func TestActionProtectedFirstDisjunct(t *testing.T) {
	cases := []struct {
		name   string
		action ops.ActionKind
		kind   ops.KindClass
		want   bool
	}{
		{"delete data", ops.ActionDelete, ops.KindData, true},
		{"replace iam", ops.ActionReplace, ops.KindIAM, true},
		{"delete secret", ops.ActionDelete, ops.KindSecret, true},
		{"replace crd_or_namespace", ops.ActionReplace, ops.KindCRDOrNamespace, true},
		{"delete workload (not data-like)", ops.ActionDelete, ops.KindWorkload, false},
		{"update data (not delete/replace)", ops.ActionUpdate, ops.KindData, false},
		{"create data", ops.ActionCreate, ops.KindData, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := ops.Action{Action: c.action, KindClass: c.kind, Tier: "nonprod", Kind: "SomeKind"}
			if got := ActionProtected(a, nil); got != c.want {
				t.Errorf("ActionProtected(%s/%s) = %v, want %v", c.action, c.kind, got, c.want)
			}
		})
	}
}

func TestActionProtectedSecondDisjunct(t *testing.T) {
	protectedKinds := []string{"Namespace", "aws_s3_bucket"}

	cases := []struct {
		name   string
		action ops.ActionKind
		tier   string
		kind   string
		want   bool
	}{
		{"update prod protected kind", ops.ActionUpdate, "prod", "Namespace", true},
		{"create prod protected kind", ops.ActionCreate, "prod", "aws_s3_bucket", true},
		{"noop prod protected kind never protected", ops.ActionNoop, "prod", "Namespace", false},
		{"update nonprod protected kind", ops.ActionUpdate, "nonprod", "Namespace", false},
		{"update prod unprotected kind", ops.ActionUpdate, "prod", "ConfigMap", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := ops.Action{Action: c.action, KindClass: ops.KindOther, Tier: c.tier, Kind: c.kind}
			if got := ActionProtected(a, protectedKinds); got != c.want {
				t.Errorf("ActionProtected(%s/%s/%s) = %v, want %v", c.action, c.tier, c.kind, got, c.want)
			}
		})
	}
}

func TestActionSensitive(t *testing.T) {
	cases := []struct {
		name           string
		addonSensitive bool
		kindClass      ops.KindClass
		want           bool
	}{
		{"kind_class secret alone", false, ops.KindSecret, true},
		{"addon-observed alone", true, ops.KindWorkload, true},
		{"both", true, ops.KindSecret, true},
		{"neither", false, ops.KindWorkload, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := ops.Action{Sensitive: c.addonSensitive, KindClass: c.kindClass}
			if got := ActionSensitive(a); got != c.want {
				t.Errorf("ActionSensitive() = %v, want %v", got, c.want)
			}
		})
	}
}
