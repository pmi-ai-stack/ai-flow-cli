package risk

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func TestComputeBlastRadiusExcludesNoop(t *testing.T) {
	actions := []ops.Action{
		{Action: ops.ActionNoop, Env: "prod", Kind: "Deployment", KindClass: ops.KindWorkload, Addon: "kubectl", Address: "prod/Deployment/api"},
		{Action: ops.ActionUpdate, Env: "prod", Kind: "Deployment", KindClass: ops.KindWorkload, Addon: "kubectl", Address: "prod/Deployment/api"},
	}
	br := ComputeBlastRadius(actions, nil)
	if len(br.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1 (noop excluded)", len(br.Entries))
	}
	if br.Entries[0].Count != 1 {
		t.Errorf("Count = %d, want 1", br.Entries[0].Count)
	}
	if got := br.KindClassCounts[ops.KindWorkload]; got != 1 {
		t.Errorf("KindClassCounts[workload] = %d, want 1", got)
	}
}

func TestComputeBlastRadiusDedupsAndCounts(t *testing.T) {
	actions := []ops.Action{
		{Action: ops.ActionUpdate, Env: "prod", Kind: "Deployment", KindClass: ops.KindWorkload, Addon: "kubectl", Address: "prod/Deployment/api"},
		{Action: ops.ActionCreate, Env: "prod", Kind: "Deployment", KindClass: ops.KindWorkload, Addon: "kubectl", Address: "prod/Deployment/worker"},
		{Action: ops.ActionCreate, Env: "prod", Kind: "Service", KindClass: ops.KindNetwork, Addon: "kubectl", Address: "prod/Service/api"},
	}
	br := ComputeBlastRadius(actions, nil)
	if len(br.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2 (two Deployment actions in the same namespace dedup to one entry)", len(br.Entries))
	}
	for _, e := range br.Entries {
		if e.Kind == "Deployment" && e.Count != 2 {
			t.Errorf("Deployment count = %d, want 2", e.Count)
		}
	}
	if br.KindClassCounts[ops.KindWorkload] != 2 || br.KindClassCounts[ops.KindNetwork] != 1 {
		t.Errorf("KindClassCounts = %+v, want workload:2 network:1", br.KindClassCounts)
	}
}

func TestComputeBlastRadiusScopeByAddon(t *testing.T) {
	cfg := &config.Ops{Environments: map[string]config.Environment{
		"prod": {Tier: "prod", Terraform: &config.TerraformBinding{Workspace: "prod-ws"}},
	}}

	actions := []ops.Action{
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "prod", Kind: "Deployment", KindClass: ops.KindWorkload, Address: "payments/Deployment/api"},
		{Action: ops.ActionCreate, Addon: "terraform", Env: "prod", Kind: "aws_instance", KindClass: ops.KindWorkload, Address: "aws_instance.web"},
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "", Kind: "Deployment", KindClass: ops.KindWorkload, Address: "-/Deployment/cluster-wide"},
	}
	br := ComputeBlastRadius(actions, cfg)

	scopes := map[string]string{}
	for _, e := range br.Entries {
		scopes[e.Env+"/"+e.Kind] = e.Scope
	}
	if got := scopes["prod/Deployment"]; got != "payments" {
		t.Errorf("kubectl scope = %q, want %q (namespace from Address)", got, "payments")
	}
	if got := scopes["prod/aws_instance"]; got != "prod-ws" {
		t.Errorf("terraform scope = %q, want %q (workspace from config)", got, "prod-ws")
	}
	if got := scopes["/Deployment"]; got != "-" {
		t.Errorf("unbound-env kubectl scope = %q, want %q", got, "-")
	}
}

func TestComputeBlastRadiusDeterministicOrder(t *testing.T) {
	actions := []ops.Action{
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "prod", Kind: "Service", Address: "z/Service/a"},
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "prod", Kind: "Deployment", Address: "a/Deployment/a"},
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "dev", Kind: "Deployment", Address: "a/Deployment/a"},
	}
	first := ComputeBlastRadius(actions, nil)
	for i := 0; i < 50; i++ {
		got := ComputeBlastRadius(actions, nil)
		if len(got.Entries) != len(first.Entries) {
			t.Fatalf("run %d: len(Entries) = %d, want %d", i, len(got.Entries), len(first.Entries))
		}
		for j := range got.Entries {
			if got.Entries[j] != first.Entries[j] {
				t.Fatalf("run %d: Entries[%d] = %+v, want %+v", i, j, got.Entries[j], first.Entries[j])
			}
		}
	}
	if first.Entries[0].Env != "dev" {
		t.Errorf("Entries[0].Env = %q, want %q (sorted by env first)", first.Entries[0].Env, "dev")
	}
}
