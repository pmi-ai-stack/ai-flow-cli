package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// impactTestRisk mirrors the spec's own config.json example — reused
// across this file's tests so a hand-computed expectation is checkable
// against the spec's own numbers.
var impactTestRisk = config.Risk{
	ActionWeight:   config.ActionWeight{Create: 1, Update: 2, Replace: 5, Delete: 8, Noop: 0},
	KindWeight:     config.KindWeight{Workload: 2, Network: 3, RBAC: 4, Secret: 4, Data: 6, IAM: 6, CRDOrNamespace: 5, Pipeline: 2, Other: 1},
	TierMultiplier: config.TierMultiplier{Sandbox: 0.2, Nonprod: 1, Prod: 3},
	DriftPenalty:   5,
	OfflinePenalty: 10,
}

func writePlanFixture(t *testing.T, ticketDir string, tgt ops.Target, cs ops.Changeset) {
	t.Helper()
	dir, filename := planFilePath(tgt)
	absDir := filepath.Join(ticketDir, "plans", dir)
	if _, err := handoff.WriteRaw(absDir, filename, cs); err != nil {
		t.Fatalf("write plan fixture: %v", err)
	}
}

func writeDryRunResultsTargets(t *testing.T, home, key, mode string, targets []opsDryRunArtifactTarget) {
	t.Helper()
	ticketDir := filepath.Join(home, key)
	if _, err := handoff.WriteArtifact(ticketDir, "dryrun-results.json", "dryrun-results", opsDryRunArtifact{Mode: mode, Targets: targets}); err != nil {
		t.Fatalf("write dryrun-results.json: %v", err)
	}
}

func TestLoadTargetChangesetsNoDryRunResultsIsNil(t *testing.T) {
	home := t.TempDir()
	tcs, mode, err := loadTargetChangesets(filepath.Join(home, "PROJ-1"))
	if err != nil {
		t.Fatal(err)
	}
	if tcs != nil || mode != "" {
		t.Errorf("tcs = %v, mode = %q, want nil, \"\"", tcs, mode)
	}
}

func TestLoadTargetChangesetsJoinsAcrossBothFiles(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	ticketDir := filepath.Join(home, key)
	tgt := ops.Target{Addon: "no-such-addon", Repo: "acme.infra", Path: "infra/prod", Scope: ops.ScopeDiff}

	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})
	writeDryRunResultsTargets(t, home, key, "live", []opsDryRunArtifactTarget{
		{Hash: "h1", Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})
	cs := ops.Changeset{Actions: []ops.Action{{Address: "a", Action: ops.ActionUpdate}}}
	writePlanFixture(t, ticketDir, tgt, cs)

	tcs, mode, err := loadTargetChangesets(ticketDir)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "live" {
		t.Errorf("mode = %q, want live", mode)
	}
	if len(tcs) != 1 || tcs[0].Hash != "h1" || len(tcs[0].Changeset.Actions) != 1 {
		t.Fatalf("tcs = %+v, want one target with hash h1 and one action", tcs)
	}
}

func TestLoadTargetChangesetsNoPlanFileIsEmptyChangeset(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	ticketDir := filepath.Join(home, key)
	tgt := ops.Target{Addon: "no-such-addon", Repo: "acme.infra", Path: "infra/prod", Scope: ops.ScopeDiff}

	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})
	writeDryRunResultsTargets(t, home, key, "live", []opsDryRunArtifactTarget{
		{Hash: "h1", Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})
	// No plan file written at all — the addon had no DryRun capability.

	tcs, _, err := loadTargetChangesets(ticketDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tcs) != 1 || len(tcs[0].Changeset.Actions) != 0 {
		t.Fatalf("tcs = %+v, want one target with an empty changeset", tcs)
	}
}

func TestLoadTargetChangesetsUnknownTargetIsError(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	ticketDir := filepath.Join(home, key)
	// ops-targets.json is empty, but dryrun-results.json names a target.
	writeDryRunResultsTargets(t, home, key, "live", []opsDryRunArtifactTarget{
		{Hash: "h1", Target: "acme.infra:infra/prod", Validation: []ops.Finding{}},
	})
	if _, _, err := loadTargetChangesets(ticketDir); err == nil {
		t.Fatal("want an error when dryrun-results.json names a target absent from ops-targets.json")
	}
}

func TestComputeImpactScoreAndBlastRadius(t *testing.T) {
	tgt := ops.Target{Addon: "kubectl", Repo: "acme.k8s-config", Path: "overlays/eu-prod"}
	tc := targetChangeset{
		Target: tgt,
		Changeset: ops.Changeset{Actions: []ops.Action{
			{Address: "prod/Deployment/api", Addon: "kubectl", Env: "prod", Tier: "prod", Kind: "Deployment", KindClass: ops.KindWorkload, Action: ops.ActionUpdate, Target: ops.TargetKey(tgt)},
		}},
	}
	opsCfg := &config.Ops{Risk: impactTestRisk}

	artifact := computeImpact([]targetChangeset{tc}, nil, "live", opsCfg)

	// action_weight[update]=2 * kind_weight[workload]=2 * tier_multiplier_per_mille[prod]=3000 = 12000 -> score 12.
	if artifact.Score != 12 {
		t.Errorf("Score = %d, want 12", artifact.Score)
	}
	if len(artifact.Contributions) != 1 || artifact.Contributions[0].PerMille != 12000 {
		t.Errorf("Contributions = %+v, want one entry with PerMille 12000", artifact.Contributions)
	}
	if len(artifact.BlastRadius.Entries) != 1 || artifact.BlastRadius.Entries[0].Count != 1 {
		t.Errorf("BlastRadius.Entries = %+v, want one entry with count 1", artifact.BlastRadius.Entries)
	}
	if artifact.BlastRadius.KindClassCounts["workload"] != 1 {
		t.Errorf("KindClassCounts = %+v, want workload:1", artifact.BlastRadius.KindClassCounts)
	}
	if len(artifact.ProtectedActions) != 0 {
		t.Errorf("ProtectedActions = %+v, want none (a plain update is not protected)", artifact.ProtectedActions)
	}
}

func TestComputeImpactProtectedActionIncluded(t *testing.T) {
	tgt := ops.Target{Addon: "terraform", Repo: "acme.infra", Path: "infra/prod"}
	tc := targetChangeset{
		Target: tgt,
		Changeset: ops.Changeset{Actions: []ops.Action{
			{Address: "aws_s3_bucket.data", Addon: "terraform", Env: "prod", Tier: "prod", Kind: "aws_s3_bucket", KindClass: ops.KindData, Action: ops.ActionDelete, Target: ops.TargetKey(tgt)},
		}},
	}
	opsCfg := &config.Ops{Risk: impactTestRisk}

	artifact := computeImpact([]targetChangeset{tc}, nil, "live", opsCfg)
	if len(artifact.ProtectedActions) != 1 {
		t.Fatalf("ProtectedActions = %+v, want one protected action (delete on kind_class data)", artifact.ProtectedActions)
	}
	if !artifact.ProtectedActions[0].Protected {
		t.Error("ProtectedActions[0].Protected = false, want true")
	}
}

func TestComputeImpactDriftAndOfflineCounts(t *testing.T) {
	t1 := ops.Target{Addon: "kubectl", Repo: "acme.k8s-config", Path: "a"}
	t2 := ops.Target{Addon: "kubectl", Repo: "acme.k8s-config", Path: "b"}
	tcs := []targetChangeset{
		{Target: t1, Changeset: ops.Changeset{}},
		{Target: t2, Changeset: ops.Changeset{}},
	}
	drift := map[string]opsDriftResultTarget{
		ops.TargetKey(t1): {DriftReport: ops.DriftReport{Drift: ops.DriftPresent}, Target: ops.TargetKey(t1)},
		ops.TargetKey(t2): {DriftReport: ops.DriftReport{Drift: ops.DriftOffline}, Target: ops.TargetKey(t2)},
	}
	opsCfg := &config.Ops{Risk: impactTestRisk}

	artifact := computeImpact(tcs, drift, "live", opsCfg)
	if artifact.TargetsWithDrift != 1 {
		t.Errorf("TargetsWithDrift = %d, want 1", artifact.TargetsWithDrift)
	}
	if artifact.TargetsOffline != 1 {
		t.Errorf("TargetsOffline = %d, want 1", artifact.TargetsOffline)
	}
	// score = round((1000*(5*1 + 10*1))/1000) = 15.
	if artifact.Score != 15 {
		t.Errorf("Score = %d, want 15 (drift+offline penalties only)", artifact.Score)
	}
}

func TestComputeImpactOfflineModeUnionedNotDoubleCounted(t *testing.T) {
	t1 := ops.Target{Addon: "kubectl", Repo: "acme.k8s-config", Path: "a"}
	tcs := []targetChangeset{{Target: t1, Changeset: ops.Changeset{}}}
	drift := map[string]opsDriftResultTarget{
		ops.TargetKey(t1): {DriftReport: ops.DriftReport{Drift: ops.DriftOffline}, Target: ops.TargetKey(t1)},
	}
	opsCfg := &config.Ops{Risk: impactTestRisk}

	artifact := computeImpact(tcs, drift, "offline", opsCfg)
	if artifact.TargetsOffline != 1 {
		t.Errorf("TargetsOffline = %d, want 1 (drift-offline and mode-offline union to the same one target)", artifact.TargetsOffline)
	}
}

// TestOpsImpactCommandWritesArtifact drives the real `ops impact`
// command end to end against hand-written dryrun-results.json/
// ops-targets.json/plans fixtures.
func TestOpsImpactCommandWritesArtifact(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	ticketDir := filepath.Join(home, key)
	tgt := ops.Target{Addon: "no-such-addon", Repo: "acme.infra", Path: "infra/prod", Scope: ops.ScopeDiff}

	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})
	writeDryRunResultsTargets(t, home, key, "live", []opsDryRunArtifactTarget{
		{Hash: "h1", Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})
	writePlanFixture(t, ticketDir, tgt, ops.Changeset{Actions: []ops.Action{
		{Address: "a", Addon: "no-such-addon", Env: "", Tier: "nonprod", Kind: "Thing", KindClass: ops.KindOther, Action: ops.ActionCreate, Target: ops.TargetKey(tgt)},
	}})

	stdout, _, ec := runRoot(t, "--home", home, "ops", "impact", key)
	if ec != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", ec, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "impact-results.json"))
	if err != nil {
		t.Fatalf("impact-results.json not written: %v", err)
	}
	var artifact opsImpactArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Score < 0 {
		t.Errorf("Score = %d, want >= 0", artifact.Score)
	}
}

func TestOpsImpactCatalogRegistration(t *testing.T) {
	found := false
	for _, c := range handoff.Catalog() {
		if c.Name == "ops impact" {
			found = true
			if !c.TicketScoped {
				t.Error(`Catalog "ops impact" TicketScoped = false, want true`)
			}
		}
	}
	if !found {
		t.Fatal(`Catalog() is missing "ops impact"`)
	}
}
