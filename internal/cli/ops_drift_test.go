package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/gitops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/testdata/fixtureaddon"
)

// driftFixtureTicket builds a repository with a real "infra/prod/main.tf"
// on disk for both the branch checkout and the base worktree
// (gitops.BaseWorktreePath) — driftOneTarget always renders both — and
// the matching repoArt/ops.Target pair.
func driftFixtureTicket(t *testing.T, home, key string) (repoArt *repoArtifact, tgt ops.Target) {
	t.Helper()
	repoArt, tgt = dryRunFixtureTicket(t)
	writeTerraformProdSource(t, gitops.BaseWorktreePath(home, key, "acme.infra"))
	return repoArt, tgt
}

// TestDriftOneTargetWritesValidationAndDrift proves the full pipe: both
// refs rendered, Validate called on the branch rendering, Drift called
// with the base rendering, both reported faithfully.
func TestDriftOneTargetWritesValidationAndDrift(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := driftFixtureTicket(t, home, key)

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.ValidateFindings = []ops.Finding{{Tool: "tflint", Severity: "error", Message: "boom"}}
	addon.DriftReport = &ops.DriftReport{Drift: ops.DriftPresent, Resources: []string{"aws_s3_bucket.x"}}
	reg := ops.NewRegistry(addon)

	rt, err := driftOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg)
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil {
		t.Fatal("driftOneTarget = nil, want a result")
	}
	if rt.Drift != ops.DriftPresent || len(rt.Resources) != 1 {
		t.Errorf("drift = %+v, want DriftPresent with one resource", rt.DriftReport)
	}
	if len(rt.Validation) != 1 || rt.Validation[0].Message != "boom" {
		t.Errorf("validation = %+v, want the addon's own finding", rt.Validation)
	}
	if rt.Hash == "" {
		t.Error("hash is empty, want the branch tree hash")
	}
}

// TestDriftOneTargetDegradesToOfflineOnMissingCredentials proves
// render-drift never raises live_credentials_missing: an addon that
// cannot reach live state reports drift: offline instead.
func TestDriftOneTargetDegradesToOfflineOnMissingCredentials(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := driftFixtureTicket(t, home, key)
	envName := "prod"
	tgt.Environment = &envName

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.LiveUnreachable = true
	reg := ops.NewRegistry(addon)

	rt, err := driftOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg)
	if err != nil {
		t.Fatalf("driftOneTarget returned an error, want a degraded offline result: %v", err)
	}
	if rt.Drift != ops.DriftOffline {
		t.Errorf("drift = %q, want offline", rt.Drift)
	}
}

// TestDriftOneTargetNotApplicableWithoutDriftCapability proves an
// addon with no Drift capability (github-actions, in the real matrix)
// gets not_applicable and no live call at all.
func TestDriftOneTargetNotApplicableWithoutDriftCapability(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := driftFixtureTicket(t, home, key)

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.Caps.Drift = false
	reg := ops.NewRegistry(addon)

	rt, err := driftOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Drift != ops.DriftNotApplicable {
		t.Errorf("drift = %q, want not_applicable", rt.Drift)
	}
}

// TestDriftOneTargetUnregisteredAddonIsANoop mirrors renderOneTarget's
// own precedent.
func TestDriftOneTargetUnregisteredAddonIsANoop(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := driftFixtureTicket(t, home, key)
	reg := ops.NewRegistry() // empty — the addon is not registered

	rt, err := driftOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg)
	if err != nil {
		t.Fatal(err)
	}
	if rt != nil {
		t.Errorf("driftOneTarget = %+v, want nil (unregistered addon)", rt)
	}
}

func TestOpsDriftUnregisteredAddonIsANoop(t *testing.T) {
	home, _ := terraformTargetTicket(t, "PROJ-1")
	// terraformTargetTicket's own target has changed: nil (scope repo),
	// so it would not qualify by default — override with a changed one.
	writeChangedTerraformTarget(t, home, "PROJ-1", true)

	stdout, _, ec := runRoot(t, "--home", home, "ops", "drift", "PROJ-1")
	if ec != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", ec, stdout.String())
	}
}

func TestOpsDriftMissingOpsTargetsIsMissingArtifact(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoFixture(t, filepath.Join(home, "PROJ-1"), "Team Beta", nil)

	stdout, _, ec := runRoot(t, "--home", home, "ops", "drift", "PROJ-1")
	if ec != 3 {
		t.Fatalf("exitCode = %d, want 3 (missing_artifact)\n%s", ec, stdout.String())
	}
}

func TestOpsDriftCatalogRegistration(t *testing.T) {
	found := false
	for _, c := range handoff.Catalog() {
		if c.Name == "ops drift" {
			found = true
			if !c.TicketScoped {
				t.Error(`Catalog "ops drift" TicketScoped = false, want true`)
			}
		}
	}
	if !found {
		t.Fatal(`Catalog() is missing "ops drift"`)
	}
}
