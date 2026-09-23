package cli

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

// TestCheckDryRunFreshnessNoTargetsIsNil proves the tolerant-of-absence
// shape: no ops-targets.json at all is nothing to check.
func TestCheckDryRunFreshnessNoTargetsIsNil(t *testing.T) {
	home := t.TempDir()
	if err := checkDryRunFreshness(Flags{Home: home}, "PROJ-1"); err != nil {
		t.Errorf("err = %v, want nil (no ops-targets.json)", err)
	}
}

// TestCheckDryRunFreshnessNoDryRunResultsIsNil proves the same
// tolerance for a missing dryrun-results.json — nothing to compare
// against yet.
func TestCheckDryRunFreshnessNoDryRunResultsIsNil(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)
	repoArt.Repositories[0].InstructionsFile = "AGENTS.md"
	writeRepoFixture(t, filepath.Join(home, key), "Team Beta", repoArt.Repositories)
	changed := true
	tgt.Changed = &changed
	tgt.Scope = ops.ScopeDiff
	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})

	if err := checkDryRunFreshness(Flags{Home: home}, key); err != nil {
		t.Errorf("err = %v, want nil (no dryrun-results.json)", err)
	}
}

// TestCheckDryRunFreshnessFiresOnStaleHashThenClears is the plan's own
// "the tree-hash refusal firing and clearing" test: a dryrun-results.json
// recording a hash that no longer matches the live checkout refuses;
// re-running `ops dry-run` (simulated here by writing the real current
// hash) clears the refusal.
func TestCheckDryRunFreshnessFiresOnStaleHashThenClears(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)
	repoArt.Repositories[0].InstructionsFile = "AGENTS.md"
	writeRepoFixture(t, filepath.Join(home, key), "Team Beta", repoArt.Repositories)
	changed := true
	tgt.Changed = &changed
	tgt.Scope = ops.ScopeDiff
	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})

	writeDryRunResultsFixture(t, home, key, []opsDryRunArtifactTarget{
		{Hash: "stale-hash-not-matching-the-real-checkout", Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})

	err := checkDryRunFreshness(Flags{Home: home}, key)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "stage_not_allowed" {
		t.Fatalf("err = %v, want a *envelope.Error with Code stage_not_allowed", err)
	}

	// Clear it: recompute the real hash and rewrite dryrun-results.json,
	// exactly what a real `ops dry-run KEY` re-run would do.
	sourceDir, err := resolveTargetSourceDir(Flags{Home: home}, key, repoArt, tgt, ops.RefBranch)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := render.TreeHash(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	writeDryRunResultsFixture(t, home, key, []opsDryRunArtifactTarget{
		{Hash: hash, Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})

	if err := checkDryRunFreshness(Flags{Home: home}, key); err != nil {
		t.Errorf("err = %v, want nil after refreshing dryrun-results.json with the current hash", err)
	}
}

// TestCheckDryRunFreshnessIgnoresUnchangedTargets proves only targets
// with changed: true are ever compared.
func TestCheckDryRunFreshnessIgnoresUnchangedTargets(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)
	repoArt.Repositories[0].InstructionsFile = "AGENTS.md"
	writeRepoFixture(t, filepath.Join(home, key), "Team Beta", repoArt.Repositories)
	unchanged := false
	tgt.Changed = &unchanged
	tgt.Scope = ops.ScopeDiff
	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})
	writeDryRunResultsFixture(t, home, key, []opsDryRunArtifactTarget{
		{Hash: "whatever-stale-value", Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})

	if err := checkDryRunFreshness(Flags{Home: home}, key); err != nil {
		t.Errorf("err = %v, want nil (target is not changed, never compared)", err)
	}
}

// writeDryRunResultsFixture writes home/key/dryrun-results.json
// directly.
func writeDryRunResultsFixture(t *testing.T, home, key string, targets []opsDryRunArtifactTarget) {
	t.Helper()
	ticketDir := filepath.Join(home, key)
	if _, err := handoff.WriteArtifact(ticketDir, "dryrun-results.json", "dryrun-results", opsDryRunArtifact{Mode: "live", Targets: targets}); err != nil {
		t.Fatalf("write dryrun-results.json: %v", err)
	}
}
