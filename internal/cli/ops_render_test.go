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

// terraformTargetTicket builds a ticket whose ops-targets.json names
// one target ("acme.infra:infra/prod", addon "terraform" — unregistered
// in DefaultRegistry() before phase 21) and whose repo.json points
// repository "acme.infra" at repoDir. Mirrors internal/schema's own
// ops-targets valid fixture (schema/testdata/ops-targets/valid.json),
// so `--target acme.infra:infra/prod` always has something real to
// match against without needing an addon to actually be registered.
func terraformTargetTicket(t *testing.T, key string) (home, repoDir string) {
	t.Helper()
	home = t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	repoDir = t.TempDir()
	ticketDir := filepath.Join(home, key)
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.infra", LocalPath: repoDir, InstructionsFile: "AGENTS.md"},
	})

	targets := ops.DetectResult{
		AppFiles: []string{},
		Targets:  []ops.Target{{Addon: "terraform", Repo: "acme.infra", Path: "infra/prod", Scope: "repo", Files: []string{}}},
	}
	if _, err := handoff.WriteArtifact(ticketDir, "ops-targets.json", "ops-targets", targets); err != nil {
		t.Fatalf("write ops-targets.json: %v", err)
	}
	return home, repoDir
}

func opsRenderEnvelope(t *testing.T, stdout []byte) (ok bool, code string, targets []struct {
	Target string `json:"target"`
	Hash   string `json:"hash"`
	Dir    string `json:"dir"`
}) {
	t.Helper()
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		Result struct {
			Targets []struct {
				Target string `json:"target"`
				Hash   string `json:"hash"`
				Dir    string `json:"dir"`
			} `json:"targets"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout)
	}
	return env.OK, env.Error.Code, env.Result.Targets
}

func TestOpsRenderBadRefIsUsageError(t *testing.T) {
	home, _ := terraformTargetTicket(t, "PROJ-1")
	stdout, _, ec := runRoot(t, "--home", home, "ops", "render", "PROJ-1", "--ref", "bogus")
	if ec != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", ec, stdout.String())
	}
	ok, code, _ := opsRenderEnvelope(t, stdout.Bytes())
	if ok || code != "usage" {
		t.Errorf("ok=%v code=%q, want ok=false code=usage", ok, code)
	}
}

func TestOpsRenderMissingOpsTargetsIsMissingArtifact(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoFixture(t, filepath.Join(home, "PROJ-1"), "Team Beta", nil)

	stdout, _, ec := runRoot(t, "--home", home, "ops", "render", "PROJ-1", "--ref", "branch")
	if ec != 3 {
		t.Fatalf("exitCode = %d, want 3 (missing_artifact)\n%s", ec, stdout.String())
	}
	ok, code, _ := opsRenderEnvelope(t, stdout.Bytes())
	if ok || code != "missing_artifact" {
		t.Errorf("ok=%v code=%q, want ok=false code=missing_artifact", ok, code)
	}
}

func TestOpsRenderUnknownTargetIsMissingArtifact(t *testing.T) {
	home, _ := terraformTargetTicket(t, "PROJ-1")
	stdout, _, ec := runRoot(t, "--home", home, "ops", "render", "PROJ-1", "--ref", "branch", "--target", "no-such:target")
	if ec != 3 {
		t.Fatalf("exitCode = %d, want 3 (missing_artifact)\n%s", ec, stdout.String())
	}
	ok, code, _ := opsRenderEnvelope(t, stdout.Bytes())
	if ok || code != "missing_artifact" {
		t.Errorf("ok=%v code=%q, want ok=false code=missing_artifact", ok, code)
	}
}

// TestOpsRenderUnregisteredAddonIsANoop proves DefaultRegistry()
// shipping empty (no addon before phase 21) makes `ops render` a
// harmless no-op even when ops-targets.json names a real target — the
// same "not a stub, the correct answer today" property phase 18
// already established for `ops detect`/`ops addons`.
func TestOpsRenderUnregisteredAddonIsANoop(t *testing.T) {
	home, _ := terraformTargetTicket(t, "PROJ-1")
	stdout, _, ec := runRoot(t, "--home", home, "ops", "render", "PROJ-1", "--ref", "branch", "--target", "acme.infra:infra/prod")
	if ec != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", ec, stdout.String())
	}
	ok, _, targets := opsRenderEnvelope(t, stdout.Bytes())
	if !ok {
		t.Fatalf("ok = false, want true\n%s", stdout.String())
	}
	if len(targets) != 0 {
		t.Errorf("targets = %v, want none (terraform is not registered before phase 21)", targets)
	}
}

func TestOpsRenderCatalogRegistration(t *testing.T) {
	found := false
	for _, c := range handoff.Catalog() {
		if c.Name == "ops render" {
			found = true
			if !c.TicketScoped {
				t.Error(`Catalog "ops render" TicketScoped = false, want true`)
			}
		}
	}
	if !found {
		t.Fatal(`Catalog() is missing "ops render"`)
	}
}

// The remaining tests call renderOneTarget directly, injecting a
// fixture addon registry — internal/ops/render's own tests already
// prove Render's real behavior (cache reuse, redaction, mode 0700);
// these prove this package's own orchestration (source-dir resolution,
// error wrapping) that only ever runs once a real addon is registered,
// which DefaultRegistry() never is before phase 21 (see the phase-19
// plan's design decision on why renderOneTarget takes an explicit
// *ops.Registry parameter).

func TestRenderOneTargetUnknownRepoIsMissingArtifact(t *testing.T) {
	home := t.TempDir()
	reg := ops.NewRegistry(fixtureaddon.New("terraform", "infra/prod"))
	repoArt := &repoArtifact{Repositories: nil}
	tgt := ops.Target{Addon: "terraform", Repo: "acme.infra", Path: "infra/prod"}

	_, err := renderOneTarget(context.Background(), Flags{Home: home}, "PROJ-1", repoArt, tgt, ops.RefBranch, reg)
	assertMissingArtifact(t, err)
}

func TestRenderOneTargetNoBaseWorktreeIsMissingArtifact(t *testing.T) {
	home := t.TempDir()
	repoDir := t.TempDir()
	reg := ops.NewRegistry(fixtureaddon.New("terraform", "infra/prod"))
	repoArt := &repoArtifact{Repositories: []repoArtifactRepository{{Name: "acme.infra", LocalPath: repoDir}}}
	tgt := ops.Target{Addon: "terraform", Repo: "acme.infra", Path: "infra/prod"}

	_, err := renderOneTarget(context.Background(), Flags{Home: home}, "PROJ-1", repoArt, tgt, ops.RefBase, reg)
	assertMissingArtifact(t, err)
}

// branchFixtureTicket builds a repository directory with a real
// "infra/prod/main.tf" on disk and the matching repo.json-shaped
// repoArt/ops.Target pair — the setup TestRenderOneTargetBranchSuccess
// and TestRenderOneTargetSimulateWritesNothing both need, differing
// only in the registry/Flags they call renderOneTarget with.
func branchFixtureTicket(t *testing.T) (repoArt *repoArtifact, tgt ops.Target) {
	t.Helper()
	repoDir := t.TempDir()
	srcDir := filepath.Join(repoDir, "infra", "prod")
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoArt = &repoArtifact{Repositories: []repoArtifactRepository{{Name: "acme.infra", LocalPath: repoDir}}}
	tgt = ops.Target{Addon: "terraform", Repo: "acme.infra", Path: "infra/prod"}
	return repoArt, tgt
}

func TestRenderOneTargetBranchSuccess(t *testing.T) {
	home := t.TempDir()
	repoArt, tgt := branchFixtureTicket(t)
	reg := ops.NewRegistry(fixtureaddon.New("terraform", "infra/prod"))

	rt, err := renderOneTarget(context.Background(), Flags{Home: home}, "PROJ-1", repoArt, tgt, ops.RefBranch, reg)
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil || rt.Hash == "" || rt.Dir == "" {
		t.Fatalf("renderOneTarget result = %+v, want a populated hash and dir", rt)
	}
	if _, err := os.Stat(filepath.Join(home, "PROJ-1", rt.Dir, "main.tf")); err != nil {
		t.Errorf("redacted copy not written: %v", err)
	}
}

func TestRenderOneTargetSimulateWritesNothing(t *testing.T) {
	home := t.TempDir()
	repoArt, tgt := branchFixtureTicket(t)
	addon := fixtureaddon.New("terraform", "infra/prod")
	reg := ops.NewRegistry(addon)

	rt, err := renderOneTarget(context.Background(), Flags{Home: home, Simulate: true}, "PROJ-1", repoArt, tgt, ops.RefBranch, reg)
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil || rt.Hash != "" || rt.Dir != "" {
		t.Errorf("renderOneTarget --simulate result = %+v, want a preview with no hash/dir", rt)
	}
	if addon.RenderCalls != 0 {
		t.Errorf("RenderCalls = %d, want 0 under --simulate", addon.RenderCalls)
	}
	if _, err := os.Stat(filepath.Join(home, "cache")); !os.IsNotExist(err) {
		t.Errorf("--simulate must not create %s/cache", home)
	}
}

func assertMissingArtifact(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("err = nil, want missing_artifact")
	}
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "missing_artifact" {
		t.Errorf("err = %v, want a *envelope.Error with Code missing_artifact", err)
	}
}
