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
	for i := range 50 {
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

// TestComputeBlastRadiusSortTiebreakByScopeThenKind pins the full
// three-key sort — (env, scope, kind) — rather than only checking
// index 0 the way TestComputeBlastRadiusDeterministicOrder already
// does: two entries share Env "prod", so the ordering between them can
// only come from the Scope tie-break, and two further entries share
// both Env "prod" and Scope "a", so their ordering can only come from
// the Kind tie-break. Found necessary by phase 30's own gremlins run
// (`.ai/phase-30/implementation-log.md`) — the pre-existing test's
// single assertion left every `<` in the sort comparator's Scope and
// Kind tie-break branches free to flip to `<=`/`>`/`>=` without a test
// ever failing.
func TestComputeBlastRadiusSortTiebreakByScopeThenKind(t *testing.T) {
	actions := []ops.Action{
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "prod", Kind: "Service", Address: "b/Service/x"},
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "prod", Kind: "Deployment", Address: "b/Deployment/x"},
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "prod", Kind: "Service", Address: "a/Service/x"},
	}
	br := ComputeBlastRadius(actions, nil)
	if len(br.Entries) != 3 {
		t.Fatalf("len(Entries) = %d, want 3", len(br.Entries))
	}
	want := []struct{ Scope, Kind string }{
		{"a", "Service"},    // Scope "a" sorts before "b" regardless of Kind.
		{"b", "Deployment"}, // Same Scope "b": Kind "Deployment" sorts before "Service".
		{"b", "Service"},
	}
	for i, w := range want {
		if br.Entries[i].Scope != w.Scope || br.Entries[i].Kind != w.Kind {
			t.Errorf("Entries[%d] = {Scope:%q Kind:%q}, want {Scope:%q Kind:%q}",
				i, br.Entries[i].Scope, br.Entries[i].Kind, w.Scope, w.Kind)
		}
	}
}

// TestComputeBlastRadiusScopeForAddressWithNoLeadingSegment kills the
// idx > 0 boundary mutant at blastradius.go's own kubectl/kustomize/helm
// case: strings.Index returns 0, not -1, when Address itself starts
// with "/" (a malformed but not-impossible address). Only `> 0` (not
// `>= 0`) correctly takes the case's own unconditional "-" fallback;
// a mutated `>= 0` would instead slice the empty leading segment
// (a.Address[:0] == "", which is != "-") and wrongly return "".
func TestComputeBlastRadiusScopeForAddressWithNoLeadingSegment(t *testing.T) {
	actions := []ops.Action{
		{Action: ops.ActionCreate, Addon: "kubectl", Env: "prod", Kind: "Deployment", Address: "/Deployment/x"},
	}
	br := ComputeBlastRadius(actions, nil)
	if len(br.Entries) != 1 || br.Entries[0].Scope != "-" {
		t.Fatalf("Entries = %+v, want one entry with Scope %q (idx==0 is not > 0, so the case's own \"-\" fallback applies)", br.Entries, "-")
	}
}

// TestComputeBlastRadiusScopeForJenkins exercises scopeFor's jenkins
// case, untouched by every other test in this file — both the
// configured-URL path and the no-binding fallback.
func TestComputeBlastRadiusScopeForJenkins(t *testing.T) {
	cfg := &config.Ops{Environments: map[string]config.Environment{
		"prod": {Tier: "prod", Jenkins: &config.JenkinsBinding{URL: "https://ci.example.com/job/deploy"}},
		"dev":  {Tier: "nonprod"},
	}}
	actions := []ops.Action{
		{Action: ops.ActionCreate, Addon: "jenkins", Env: "prod", Kind: "pipeline", Address: "deploy"},
		{Action: ops.ActionCreate, Addon: "jenkins", Env: "dev", Kind: "pipeline", Address: "deploy"},
	}
	br := ComputeBlastRadius(actions, cfg)
	scopes := map[string]string{}
	for _, e := range br.Entries {
		scopes[e.Env] = e.Scope
	}
	if got := scopes["prod"]; got != "https://ci.example.com/job/deploy" {
		t.Errorf("jenkins scope with a configured URL = %q, want the URL", got)
	}
	if got := scopes["dev"]; got != "dev" {
		t.Errorf("jenkins scope with no Jenkins binding = %q, want the env name fallback %q", got, "dev")
	}
}
