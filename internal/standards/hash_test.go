package standards

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// resolveWith is the shared harness for the hash tests: one resolution
// over a freshly staged home and repository checkout, with callers able
// to mutate either before it runs.
func resolveWith(t *testing.T, mutate func(home, repoDir string, in *ResolveInput)) *Effective {
	t.Helper()
	home := stageHome(t)
	repoDir := stageRepo(t, "one", true)

	waivers, err := LoadWaivers("acme.billing.api", repoDir)
	if err != nil {
		t.Fatalf("LoadWaivers: %v", err)
	}
	in := ResolveInput{
		Home: home, Cfg: layeredConfig(6000), Team: "Platform",
		Repos:     []RepoRef{{Name: "acme.billing.api", LocalPath: repoDir, InstructionsFile: "AGENTS.md"}},
		Languages: []string{"go"}, Waivers: waivers, BudgetTokens: 6000,
		Now: fixedClock(beforeWaiverExpiry),
	}
	if mutate != nil {
		mutate(home, repoDir, &in)
	}

	e, packs, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	t.Cleanup(func() { ClosePacks(packs) })
	return e
}

// TestResolutionHashStableAcrossUnrelatedChanges is the spec's own
// "resolution hash stability across unrelated changes" test. The two
// halves are what make the hash useful: it must not move for things
// the resolution does not depend on, and it must move for everything
// it does.
func TestResolutionHashStableAcrossUnrelatedChanges(t *testing.T) {
	base := resolveWith(t, nil).Hash

	unrelated := map[string]func(home, repoDir string, in *ResolveInput){
		"a different ticket's clock": func(_, _ string, in *ResolveInput) {
			in.Now = fixedClock(beforeWaiverExpiry.AddDate(0, 0, 1))
		},
		"an unrelated config default": func(_, _ string, in *ResolveInput) {
			in.Cfg.Defaults = config.Defaults{Viewport: "1920x1080", MutationThreshold: 95}
		},
		"a reworded waiver reason": func(_, _ string, in *ResolveInput) {
			for i := range in.Waivers {
				in.Waivers[i].Reason = "reworded, same check, same expiry"
			}
		},
		"an untracked file in the repository checkout": func(_, repoDir string, _ *ResolveInput) {
			writeFile(t, filepath.Join(repoDir, "main.go"), "package main\n")
		},
	}
	for name, mutate := range unrelated {
		t.Run("stable: "+name, func(t *testing.T) {
			if got := resolveWith(t, mutate).Hash; got != base {
				t.Errorf("hash moved on an unrelated change (%s): %s -> %s", name, base, got)
			}
		})
	}

	related := map[string]func(home, repoDir string, in *ResolveInput){
		"a changed budget": func(_, _ string, in *ResolveInput) {
			in.BudgetTokens = 3000
		},
		"a new language": func(_, _ string, in *ResolveInput) {
			in.Languages = append(in.Languages, "typescript")
		},
		"a new waiver": func(_, _ string, in *ResolveInput) {
			in.Waivers = append(in.Waivers, Waiver{
				CheckID: "org.lint", Reason: "r", Author: "dev",
				Created: "2026-01-01T00:00:00Z", Expires: "2099-01-01T00:00:00Z", Repo: "acme.billing.api",
			})
		},
		"edited pack content under the same pinned ref": func(home, _ string, _ *ResolveInput) {
			writeFile(t, filepath.Join(CacheDir(home, "acme-org", "v3.2.0"), "guidance", "errors.md"), "# Org errors\n\nRewritten.\n")
		},
		"a changed instructions file": func(_, repoDir string, _ *ResolveInput) {
			writeFile(t, filepath.Join(repoDir, "AGENTS.md"), "# acme.billing.api\n\nRewritten.\n")
		},
	}
	for name, mutate := range related {
		t.Run("moves: "+name, func(t *testing.T) {
			if got := resolveWith(t, mutate).Hash; got == base {
				t.Errorf("hash did not move on a real change (%s): still %s", name, got)
			}
		})
	}
}

// TestResolutionHashCoversTheSpecsOwnInputs pins the hash pre-image to
// the spec's list — languages, addons, pack commits, waivers, budget —
// so a later phase cannot quietly widen or narrow it.
func TestResolutionHashCoversTheSpecsOwnInputs(t *testing.T) {
	e := &Effective{
		Languages: []string{"go"}, Addons: []string{}, BudgetTokens: 6000,
		Packs:   []*Pack{{ID: "acme-org", Layer: LayerOrg, Ref: "v3.2.0", Commit: "abc1234", Fingerprint: "sha256:aaaa"}},
		Waivers: []WaiverRecord{{Waiver: Waiver{CheckID: "org.deps", Repo: "r", Expires: "2026-12-01T00:00:00Z"}}},
	}
	base := resolutionHash(e)

	for name, mutate := range map[string]func(*Effective){
		"languages": func(e *Effective) { e.Languages = []string{"python"} },
		"addons":    func(e *Effective) { e.Addons = []string{"terraform"} },
		"budget":    func(e *Effective) { e.BudgetTokens = 1 },
		"commit":    func(e *Effective) { e.Packs[0].Commit = "def5678" },
		"waiver expiry": func(e *Effective) {
			e.Waivers[0].Expires = "2027-12-01T00:00:00Z"
		},
	} {
		t.Run(name, func(t *testing.T) {
			clone := *e
			clone.Languages = append([]string(nil), e.Languages...)
			clone.Addons = append([]string(nil), e.Addons...)
			clone.Packs = []*Pack{{}}
			*clone.Packs[0] = *e.Packs[0]
			clone.Waivers = append([]WaiverRecord(nil), e.Waivers...)
			mutate(&clone)
			if resolutionHash(&clone) == base {
				t.Errorf("changing %s did not move the resolution hash", name)
			}
		})
	}
}

// TestFingerprintIsContentAddressed proves the fingerprint depends on
// content only — two independently staged copies of the same fixture
// fingerprint identically, and editing one byte changes it.
func TestFingerprintIsContentAddressed(t *testing.T) {
	a := stageRepo(t, "one", true)
	b := stageRepo(t, "one", true)

	packA, err := OpenPack(LayerRepo, "r", "workdir", "", "", filepath.Join(a, RepoPackDir))
	if err != nil {
		t.Fatalf("OpenPack(a): %v", err)
	}
	defer packA.Close()
	packB, err := OpenPack(LayerRepo, "r", "workdir", "", "", filepath.Join(b, RepoPackDir))
	if err != nil {
		t.Fatalf("OpenPack(b): %v", err)
	}
	defer packB.Close()

	if packA.Fingerprint != packB.Fingerprint {
		t.Errorf("two copies of identical content fingerprint differently: %s vs %s", packA.Fingerprint, packB.Fingerprint)
	}

	writeFile(t, filepath.Join(b, RepoPackDir, "guidance", "errors.md"), "# Repo errors\n\nOne byte different.\n")
	packC, err := OpenPack(LayerRepo, "r", "workdir", "", "", filepath.Join(b, RepoPackDir))
	if err != nil {
		t.Fatalf("OpenPack(c): %v", err)
	}
	defer packC.Close()
	if packC.Fingerprint == packA.Fingerprint {
		t.Error("editing a pack file did not change its fingerprint")
	}
}
