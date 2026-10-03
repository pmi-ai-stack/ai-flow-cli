package cli

import (
	"errors"
	"os"
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
	hash, err := render.TreeHash(sourceDir, nil)
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

// TestTreeSkipperImplementedByCopyTreeAddonsOnly pins which addons hash
// with a skip rule: exactly the three whose Render copies the source
// tree through a skip predicate. The other three read their source in
// place, so every file must stay part of their cache key.
func TestTreeSkipperImplementedByCopyTreeAddonsOnly(t *testing.T) {
	want := map[string]bool{"terraform": true, "kubectl": true, "helm": true, "kustomize": false, "github-actions": false, "jenkins": false}
	reg := buildOpsRegistry(Flags{Home: t.TempDir()}, nil)
	for name, skips := range want {
		a, ok := reg.Lookup(name)
		if !ok {
			t.Fatalf("addon %q not registered", name)
		}
		if got := render.SkipFor(a) != nil; got != skips {
			t.Errorf("SkipFor(%s) != nil = %v, want %v", name, got, skips)
		}
	}
	if skip := treeSkipFor(Flags{}, "terraform"); skip == nil || !skip(".terraform") || skip("modules") {
		t.Error("terraform's predicate must skip .terraform and keep modules")
	}
	if treeSkipFor(Flags{}, "no-such-addon") != nil {
		t.Error("an unregistered addon must hash everything (nil predicate)")
	}
}

// TestCheckDryRunFreshnessIgnoresTerraformInitCache is the freshness
// gate's half of PERF-4: dry-run recorded the hash render.Render keyed
// on (terraform's own skip rule), then a local `terraform init` wrote
// .terraform/ into the checkout. The gate must hash with the same rule
// and still pass — and still refuse after a real .tf edit.
func TestCheckDryRunFreshnessIgnoresTerraformInitCache(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)
	tgt.Addon = "terraform"
	repoArt.Repositories[0].InstructionsFile = "AGENTS.md"
	writeRepoFixture(t, filepath.Join(home, key), "Team Beta", repoArt.Repositories)
	changed := true
	tgt.Changed = &changed
	tgt.Scope = ops.ScopeDiff
	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})

	sourceDir, err := resolveTargetSourceDir(Flags{Home: home}, key, repoArt, tgt, ops.RefBranch)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := render.TreeHash(sourceDir, treeSkipFor(Flags{Home: home}, "terraform"))
	if err != nil {
		t.Fatal(err)
	}
	writeDryRunResultsFixture(t, home, key, []opsDryRunArtifactTarget{
		{Hash: recorded, Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})

	providers := filepath.Join(sourceDir, ".terraform", "providers")
	if err := os.MkdirAll(providers, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providers, "terraform-provider-aws"), []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkDryRunFreshness(Flags{Home: home}, key); err != nil {
		t.Fatalf("err = %v, want nil — .terraform/ is not render input", err)
	}

	if err := os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	var envErr *envelope.Error
	if err := checkDryRunFreshness(Flags{Home: home}, key); !errors.As(err, &envErr) || envErr.Code != "stage_not_allowed" {
		t.Fatalf("err = %v, want stage_not_allowed after editing main.tf", err)
	}
}
