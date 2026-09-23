package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/testdata/fixtureaddon"
)

// dryRunFixtureTicket builds a repository with a real
// "infra/prod/main.tf" on disk (branch only — dry-run never renders
// base) and the matching repoArt/ops.Target pair.
func dryRunFixtureTicket(t *testing.T) (repoArt *repoArtifact, tgt ops.Target) {
	t.Helper()
	repoDir := t.TempDir()
	writeTerraformProdSource(t, repoDir)
	repoArt = &repoArtifact{Repositories: []repoArtifactRepository{{Name: "acme.infra", LocalPath: repoDir}}}
	tgt = ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra/prod"}
	return repoArt, tgt
}

// writeTerraformProdSource writes a minimal "infra/prod/main.tf" under
// dir — the on-disk content dryRunFixtureTicket/driftFixtureTicket both
// need for a real tree hash, shared so drift's own two-directory
// (branch + base) variant doesn't hand-write the same content twice.
func writeTerraformProdSource(t *testing.T, dir string) {
	t.Helper()
	src := filepath.Join(dir, "infra", "prod")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.tf"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDryRunOneTargetReusesValidationWhenHashUnchanged proves the
// plan's own "re-render and re-validate every target whose tree hash
// moved" reading: an unmoved target reuses render-drift's own
// validation verbatim and never calls Addon.Validate again.
func TestDryRunOneTargetReusesValidationWhenHashUnchanged(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.ValidateFindings = []ops.Finding{{Tool: "tflint", Severity: "error", Message: "new"}}
	reg := ops.NewRegistry(addon)

	// Compute the real branch tree hash first, so the "prior" entry
	// matches it exactly — the "unchanged" case.
	hash, err := renderForTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, ops.RefBranch, addon)
	if err != nil {
		t.Fatal(err)
	}
	prior := map[string]opsDriftResultTarget{
		ops.TargetKey(tgt): {Hash: hash, Target: ops.TargetKey(tgt), Validation: []ops.Finding{{Tool: "tflint", Severity: "error", Message: "old"}}},
	}

	rt, _, err := dryRunOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg, prior, false)
	if err != nil {
		t.Fatal(err)
	}
	if addon.ValidateCalls != 0 {
		t.Errorf("ValidateCalls = %d, want 0 (hash unchanged, must reuse render-drift's own validation)", addon.ValidateCalls)
	}
	if len(rt.Validation) != 1 || rt.Validation[0].Message != "old" {
		t.Errorf("validation = %+v, want render-drift's own [\"old\"]", rt.Validation)
	}
}

// TestDryRunOneTargetRevalidatesWhenHashChanged proves the opposite
// direction: a source edit inside checks (between render-drift and
// dry-run) is re-validated by dry-run.
func TestDryRunOneTargetRevalidatesWhenHashChanged(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.ValidateFindings = []ops.Finding{{Tool: "tflint", Severity: "error", Message: "new"}}
	reg := ops.NewRegistry(addon)

	prior := map[string]opsDriftResultTarget{
		ops.TargetKey(tgt): {Hash: "stale-hash-from-before-the-edit", Target: ops.TargetKey(tgt), Validation: []ops.Finding{{Tool: "tflint", Severity: "error", Message: "old"}}},
	}

	rt, _, err := dryRunOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg, prior, false)
	if err != nil {
		t.Fatal(err)
	}
	if addon.ValidateCalls != 1 {
		t.Errorf("ValidateCalls = %d, want 1 (hash moved, must re-validate)", addon.ValidateCalls)
	}
	if len(rt.Validation) != 1 || rt.Validation[0].Message != "new" {
		t.Errorf("validation = %+v, want the fresh [\"new\"]", rt.Validation)
	}
	if rt.Hash == "stale-hash-from-before-the-edit" {
		t.Error("hash was not refreshed")
	}
}

// TestDryRunOneTargetWritesNormalizedPlan proves plans/<target>.json is
// written sorted by Address and redacted from the branch rendering's
// own markings.
func TestDryRunOneTargetWritesNormalizedPlan(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.SensitiveFiles = []string{"main.tf"} // marks the whole file content ("x") as sensitive
	addon.DryRunChangeset = &ops.Changeset{Actions: []ops.Action{
		{Address: "z_resource", Action: ops.ActionCreate},
		{Address: "a_resource", Action: ops.ActionUpdate, FieldsChanged: []string{"value=x"}},
	}}
	reg := ops.NewRegistry(addon)

	rt, at, err := dryRunOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if at.Hash != rt.Hash {
		t.Errorf("artifact target hash %q != envelope target hash %q", at.Hash, rt.Hash)
	}
	if rt.Plan == "" {
		t.Fatal("Plan path is empty")
	}

	data, err := os.ReadFile(filepath.Join(handoff.TicketDir(home, key), rt.Plan))
	if err != nil {
		t.Fatalf("plan file not written at %s: %v", rt.Plan, err)
	}
	var cs ops.Changeset
	if err := json.Unmarshal(data, &cs); err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 2 || cs.Actions[0].Address != "a_resource" || cs.Actions[1].Address != "z_resource" {
		t.Errorf("actions = %+v, want sorted by address (a_resource, z_resource)", cs.Actions)
	}
	if cs.Actions[0].FieldsChanged[0] == "value=x" {
		t.Errorf("fields_changed = %v, want the marked value redacted", cs.Actions[0].FieldsChanged)
	}

	wantDir := filepath.Join("plans", "acme.infra", "fixture", "infra", "prod.json")
	if rt.Plan != wantDir {
		t.Errorf("Plan = %q, want %q", rt.Plan, wantDir)
	}
}

// TestDryRunOneTargetOfflineSucceedsEvenWhenLiveUnreachable proves
// --offline's own signal (the unbound Env) reaches the addon.
func TestDryRunOneTargetOfflineSucceedsEvenWhenLiveUnreachable(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)
	envName := "prod"
	tgt.Environment = &envName

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.LiveUnreachable = true
	reg := ops.NewRegistry(addon)

	_, _, err := dryRunOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg, nil, true)
	if err != nil {
		t.Fatalf("dryRunOneTarget with offline=true returned an error: %v", err)
	}
}

// TestDryRunOneTargetLiveCredentialsMissingWithoutOffline proves the
// documented exit-4 mapping: a bound target whose live access fails,
// with --offline not passed, is the live_credentials_missing envelope
// error.
func TestDryRunOneTargetLiveCredentialsMissingWithoutOffline(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	repoArt, tgt := dryRunFixtureTicket(t)
	envName := "prod"
	tgt.Environment = &envName

	addon := fixtureaddon.New("fixture", "infra/prod")
	addon.LiveUnreachable = true
	reg := ops.NewRegistry(addon)

	_, _, err := dryRunOneTarget(context.Background(), Flags{Home: home}, key, repoArt, tgt, reg, nil, false)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "live_credentials_missing" {
		t.Fatalf("err = %v, want a *envelope.Error with Code live_credentials_missing", err)
	}
}

func TestOpsDryRunModeRecorded(t *testing.T) {
	home, _ := terraformTargetTicket(t, "PROJ-1")
	writeChangedTerraformTarget(t, home, "PROJ-1", true)
	writeFixture(t, home, "PROJ-1", "render-drift-results", "render-drift-results.json")

	stdout, _, ec := runRoot(t, "--home", home, "ops", "dry-run", "PROJ-1", "--offline")
	if ec != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", ec, stdout.String())
	}
	var env struct {
		Result struct {
			Mode string `json:"mode"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Result.Mode != "offline" {
		t.Errorf("mode = %q, want offline", env.Result.Mode)
	}
}

func TestOpsDryRunMissingRenderDriftResultsIsMissingArtifact(t *testing.T) {
	home, _ := terraformTargetTicket(t, "PROJ-1")
	writeChangedTerraformTarget(t, home, "PROJ-1", true)

	stdout, _, ec := runRoot(t, "--home", home, "ops", "dry-run", "PROJ-1")
	if ec != 3 {
		t.Fatalf("exitCode = %d, want 3 (missing_artifact)\n%s", ec, stdout.String())
	}
}

func TestOpsDryRunCatalogRegistration(t *testing.T) {
	found := false
	for _, c := range handoff.Catalog() {
		if c.Name == "ops dry-run" {
			found = true
			if !c.TicketScoped {
				t.Error(`Catalog "ops dry-run" TicketScoped = false, want true`)
			}
		}
	}
	if !found {
		t.Fatal(`Catalog() is missing "ops dry-run"`)
	}
}
