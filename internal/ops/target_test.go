package ops

import "testing"

// TestCheckTarget: a target's repo, addon and path become path segments
// under <KEY>/render/ and under the repository checkout, so none of
// them may escape its root (security review 2026-09-27 S10).
func TestCheckTarget(t *testing.T) {
	ok := []Target{
		{Repo: "acme.infra", Addon: "terraform", Path: "infra/prod"},
		{Repo: "acme.infra", Addon: "kubectl", Path: "."},
		{Repo: "acme.infra", Addon: "helm", Path: "charts/app/../app"},
		{Repo: "acme.k8s-config", Addon: "kustomize", Path: "overlays/eu-prod"},
	}
	for _, tg := range ok {
		if err := CheckTarget(tg); err != nil {
			t.Errorf("CheckTarget(%+v) = %v, want nil", tg, err)
		}
	}

	bad := []Target{
		{Repo: "acme.infra", Addon: "terraform", Path: "../../etc"},
		{Repo: "acme.infra", Addon: "terraform", Path: "infra/../../x"},
		{Repo: "acme.infra", Addon: "terraform", Path: "/etc"},
		{Repo: "acme.infra", Addon: "terraform", Path: ""},
		{Repo: "..", Addon: "terraform", Path: "infra"},
		{Repo: "../x", Addon: "terraform", Path: "infra"},
		{Repo: "/abs", Addon: "terraform", Path: "infra"},
		{Repo: "", Addon: "terraform", Path: "infra"},
		{Repo: ".", Addon: "terraform", Path: "infra"},
		{Repo: "acme.infra", Addon: "..", Path: "infra"},
		{Repo: "acme.infra", Addon: "", Path: "infra"},
		{Repo: "acme.infra", Addon: ".", Path: "infra"},
	}
	for _, tg := range bad {
		if err := CheckTarget(tg); err == nil {
			t.Errorf("CheckTarget(%+v) = nil, want an error", tg)
		}
	}
}
