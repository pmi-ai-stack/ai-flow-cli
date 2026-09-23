package standards

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// beforeWaiverExpiry and afterWaiverExpiry straddle the fixture
// waivers' own expires values (org.deps expires 2026-12-01, org.lint
// expired 2025-06-01), so one fixed clock exercises both the live and
// the re-fired case without either depending on the wall clock.
var (
	beforeWaiverExpiry = time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	afterWaiverExpiry  = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
)

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// stageHome lays out a --home whose standards cache already holds the
// org and team fixture packs under the ids/refs cfg names, so
// LoadLayers finds them without any network access.
func stageHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, p := range []struct{ id, ref, src string }{
		{"acme-org", "v3.2.0", "org"},
		{"platform-team", "v1.4.0", "team"},
	} {
		dst := CacheDir(home, p.id, p.ref)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.CopyFS(dst, os.DirFS(filepath.Join("testdata", "layers", p.src))); err != nil {
			t.Fatalf("CopyFS %s: %v", p.src, err)
		}
	}
	return home
}

// stageRepo copies a fixture repository checkout into a writable temp
// directory, optionally with the repo-layer pack installed under
// .ai-flow/standards/. testdata itself is never written to.
func stageRepo(t *testing.T, fixture string, withPack bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "repos", fixture))); err != nil {
		t.Fatalf("CopyFS repos/%s: %v", fixture, err)
	}
	if withPack {
		dst := filepath.Join(dir, RepoPackDir)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.CopyFS(dst, os.DirFS(filepath.Join("testdata", "layers", "repo"))); err != nil {
			t.Fatalf("CopyFS layers/repo: %v", err)
		}
	}
	return dir
}

// layeredConfig is the org+team configuration stageHome's cache
// matches: one org source, one team source under the team repo.json
// names.
func layeredConfig(budget int) *config.Config {
	return &config.Config{
		Routing: config.Routing{Teams: map[string]config.Team{
			"Platform": {Standards: []config.StandardsSource{
				{ID: "platform-team", Repo: "https://github.com/acme/platform-standards", Ref: "v1.4.0"},
			}},
		}},
		Standards: config.Standards{
			Sources:              []config.StandardsSource{{ID: "acme-org", Repo: "https://github.com/acme/standards", Ref: "v3.2.0"}},
			GuidanceBudgetTokens: budget,
		},
	}
}

// resolveFixture is the whole three-layer set resolved once, with a
// fixed clock. withPack installs the repo-layer pack; budget 0 means
// unbudgeted.
func resolveFixture(t *testing.T, withPack bool, budget int, now time.Time) (*Effective, string) {
	t.Helper()
	home := stageHome(t)
	repoDir := stageRepo(t, "one", withPack)

	waivers, err := LoadWaivers("acme.billing.api", repoDir)
	if err != nil {
		t.Fatalf("LoadWaivers: %v", err)
	}

	e, packs, err := Resolve(ResolveInput{
		Home:         home,
		Cfg:          layeredConfig(budget),
		Team:         "Platform",
		Repos:        []RepoRef{{Name: "acme.billing.api", LocalPath: repoDir, InstructionsFile: "AGENTS.md"}},
		Languages:    []string{"go"},
		Waivers:      waivers,
		BudgetTokens: budget,
		Now:          fixedClock(now),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	t.Cleanup(func() { ClosePacks(packs) })
	return e, repoDir
}

// TestResolveThreeLayersOneOverrideOneWaiver is the spec's own first
// test for this phase: "a sample org + team + repo layer set resolving
// with exactly one override and one waiver."
func TestResolveThreeLayersOneOverrideOneWaiver(t *testing.T) {
	e, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)

	// org.errors: the repo pack declares the override, so the repo
	// entry wins and the org entry is annotated.
	errorsWinner, ok := e.Section("org.errors")
	if !ok {
		t.Fatal("org.errors resolves to no effective section")
	}
	if errorsWinner.Layer != LayerRepo {
		t.Errorf("org.errors effective layer = %q, want %q (the repo pack declares the override)", errorsWinner.Layer, LayerRepo)
	}
	var shadowed *EffectiveSection
	for i := range e.Sections {
		if e.Sections[i].ID == "org.errors" && e.Sections[i].Layer == LayerOrg {
			shadowed = &e.Sections[i]
		}
	}
	if shadowed == nil {
		t.Fatal("the shadowed org copy of org.errors is not recorded at all")
	}
	if shadowed.OverriddenBy != LayerRepo {
		t.Errorf("shadowed org.errors OverriddenBy = %q, want %q", shadowed.OverriddenBy, LayerRepo)
	}
	if !strings.Contains(shadowed.OverrideReason, "trace") {
		t.Errorf("shadowed org.errors OverrideReason = %q, want the repo manifest's declared reason", shadowed.OverrideReason)
	}
	if shadowed.Conflict != "" {
		t.Errorf("shadowed org.errors Conflict = %q, want empty (the override was declared)", shadowed.Conflict)
	}

	// org.logging: the team pack replaces a `should`, which needs no
	// declaration at all.
	logging, ok := e.Section("org.logging")
	if !ok {
		t.Fatal("org.logging resolves to no effective section")
	}
	if logging.Layer != LayerTeam {
		t.Errorf("org.logging effective layer = %q, want %q (a should needs no declaration)", logging.Layer, LayerTeam)
	}

	// Exactly one live waiver: org.deps. org.lint's waiver has expired.
	var waived []string
	for _, c := range e.Checks {
		if c.Effective() && c.Waived {
			waived = append(waived, c.ID)
		}
	}
	if !slices.Equal(waived, []string{"org.deps"}) {
		t.Errorf("waived checks = %v, want [org.deps]", waived)
	}
}

// TestOrgMustWinsWhenOverrideNotDeclared is the spec's "an org must
// replaced without a reason losing to the org entry."
func TestOrgMustWinsWhenOverrideNotDeclared(t *testing.T) {
	e, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)

	// The repo pack replaces org.security (a must) without listing it
	// in overrides[], so the org entry wins.
	security, ok := e.Section("org.security")
	if !ok {
		t.Fatal("org.security resolves to no effective section")
	}
	if security.Layer != LayerOrg {
		t.Fatalf("org.security effective layer = %q, want %q (an undeclared must override loses)", security.Layer, LayerOrg)
	}

	var loser *EffectiveSection
	for i := range e.Sections {
		if e.Sections[i].ID == "org.security" && e.Sections[i].Layer == LayerRepo {
			loser = &e.Sections[i]
		}
	}
	if loser == nil {
		t.Fatal("the losing repo copy of org.security is not recorded at all")
	}
	if loser.Conflict != ConflictOverrideNotDeclared {
		t.Errorf("losing org.security Conflict = %q, want %q", loser.Conflict, ConflictOverrideNotDeclared)
	}

	want := Conflict{ID: "org.security", Kind: "section", Rule: ConflictOverrideNotDeclared, Winner: LayerOrg, Loser: LayerRepo}
	if !slices.Contains(e.Conflicts, want) {
		t.Errorf("Conflicts = %+v, want it to contain %+v", e.Conflicts, want)
	}

	// The conflict is reported, not fatal, and it reaches the envelope.
	var found bool
	for _, w := range e.Warnings() {
		if strings.Contains(w, "org.security") && strings.Contains(w, "without an overrides[] entry") {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings() = %v, want one naming org.security's undeclared override", e.Warnings())
	}
}

// TestExpiredWaiverRefires is the spec's "an expired waiver
// re-firing": the same fixture resolved on either side of an expiry
// changes exactly one check's Waived, and the expired record is still
// reported so the artifact can say why.
func TestExpiredWaiverRefires(t *testing.T) {
	live, _ := resolveFixture(t, true, 0, beforeWaiverExpiry)
	deps, ok := live.Check("org.deps")
	if !ok {
		t.Fatal("org.deps resolves to no effective check")
	}
	if !deps.Waived {
		t.Error("org.deps Waived = false before its expiry, want true")
	}

	expired, _ := resolveFixture(t, true, 0, afterWaiverExpiry)
	deps, ok = expired.Check("org.deps")
	if !ok {
		t.Fatal("org.deps resolves to no effective check")
	}
	if deps.Waived {
		t.Error("org.deps Waived = true after its expiry, want false — an expired waiver re-fires")
	}
	if deps.Waiver == nil || !deps.Waiver.Expired {
		t.Errorf("org.deps Waiver = %+v, want the expired record still attached with Expired: true", deps.Waiver)
	}

	// org.lint's waiver is already expired at both clocks.
	lint, ok := live.Check("org.lint")
	if !ok {
		t.Fatal("org.lint resolves to no effective check")
	}
	if lint.Waived {
		t.Error("org.lint Waived = true, want false — its fixture waiver expired in 2025")
	}
}

// TestSecretsCheckIsNeverWaived proves the unwaivable rule holds at the
// resolver, not only at `standards waive`: a waivers.json hand-edited
// past the command's own refusal still must not suppress the check.
func TestSecretsCheckIsNeverWaived(t *testing.T) {
	home := stageHome(t)
	repoDir := stageRepo(t, "one", false)

	e, packs, err := Resolve(ResolveInput{
		Home: home, Cfg: layeredConfig(0), Team: "Platform",
		Repos:     []RepoRef{{Name: "acme.billing.api", LocalPath: repoDir, InstructionsFile: "AGENTS.md"}},
		Languages: []string{"go"},
		Waivers: []Waiver{{
			CheckID: "org.secrets", Reason: "hand-edited past the command's refusal",
			Author: "dev", Created: "2026-01-01T00:00:00Z", Expires: "2099-01-01T00:00:00Z",
			Repo: "acme.billing.api",
		}},
		Now: fixedClock(beforeWaiverExpiry),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer ClosePacks(packs)

	secrets, ok := e.Check("org.secrets")
	if !ok {
		t.Fatal("org.secrets resolves to no effective check")
	}
	if secrets.Waived {
		t.Error("org.secrets Waived = true, want false — secrets checks can never be waived")
	}
}

// TestRepoInstructionsInjectedVerbatim proves the repo.instructions
// section carries the checkout's own file byte for byte, as a `must`
// scoped to implement and review.
func TestRepoInstructionsInjectedVerbatim(t *testing.T) {
	e, repoDir := resolveFixture(t, false, 0, beforeWaiverExpiry)

	s, ok := e.Section(RepoInstructionsSectionID)
	if !ok {
		t.Fatal("repo.instructions resolves to no effective section")
	}
	if s.Severity != "must" || s.Layer != LayerRepo {
		t.Errorf("repo.instructions = {severity: %q, layer: %q}, want {must, repo}", s.Severity, s.Layer)
	}
	if !slices.Equal(s.Section.AppliesTo.Stages, []string{"implement", "review"}) {
		t.Errorf("repo.instructions stages = %v, want [implement review]", s.Section.AppliesTo.Stages)
	}

	want, err := os.ReadFile(filepath.Join(repoDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile AGENTS.md: %v", err)
	}
	got, err := s.Pack.ReadSection(s.Section)
	if err != nil {
		t.Fatalf("ReadSection: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("repo.instructions content:\n%s\nwant:\n%s", got, want)
	}
}

// TestMultipleRepositoriesConcatenateInstructions proves the
// one-section-for-many-repositories decision: both files, in repo.json
// order, each behind an HTML-comment marker naming its source.
func TestMultipleRepositoriesConcatenateInstructions(t *testing.T) {
	home := stageHome(t)
	one := stageRepo(t, "one", false)
	two := stageRepo(t, "two", false)

	e, packs, err := Resolve(ResolveInput{
		Home: home, Cfg: layeredConfig(0), Team: "Platform",
		Repos: []RepoRef{
			{Name: "acme.billing.api", LocalPath: one, InstructionsFile: "AGENTS.md"},
			{Name: "acme.billing.web", LocalPath: two, InstructionsFile: "CLAUDE.md"},
		},
		Languages: []string{"go"},
		Now:       fixedClock(beforeWaiverExpiry),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer ClosePacks(packs)

	s, _ := e.Section(RepoInstructionsSectionID)
	body, err := s.Pack.ReadSection(s.Section)
	if err != nil {
		t.Fatalf("ReadSection: %v", err)
	}
	got := string(body)
	for _, want := range []string{
		"<!-- acme.billing.api: AGENTS.md -->",
		"Run the pre-PR command",
		"<!-- acme.billing.web: CLAUDE.md -->",
		"no class components",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("repo.instructions is missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "acme.billing.api") > strings.Index(got, "acme.billing.web") {
		t.Error("repo.instructions does not concatenate in repo.json order")
	}
}

// TestMissingRepoInstructionsExitsFour proves the loader rule's own
// exit code: a repository with neither AGENTS.md nor CLAUDE.md.
func TestMissingRepoInstructionsExitsFour(t *testing.T) {
	home := stageHome(t)
	none := stageRepo(t, "none", false)

	_, _, err := Resolve(ResolveInput{
		Home: home, Cfg: layeredConfig(0), Team: "Platform",
		Repos: []RepoRef{{Name: "acme.orphan", LocalPath: none, InstructionsFile: "AGENTS.md"}},
		Now:   fixedClock(beforeWaiverExpiry),
	})
	var envErr *envelope.Error
	if err == nil {
		t.Fatal("Resolve = nil error, want missing_repo_instructions")
	}
	if !errors.As(err, &envErr) || envErr.Code != "missing_repo_instructions" {
		t.Fatalf("Resolve error = %v, want code missing_repo_instructions", err)
	}
	ec, _ := envelope.ExitCodeFor(envErr.Code)
	if ec != envelope.ExitNeedsInput {
		t.Errorf("missing_repo_instructions maps to exit %d, want %d", ec, envelope.ExitNeedsInput)
	}
	if len(envErr.Details) != 1 || !strings.Contains(envErr.Details[0], "acme.orphan") {
		t.Errorf("Details = %v, want one entry naming the repository", envErr.Details)
	}
}

// TestPackUnreachableWhenSourceNotSynced proves the resolver never
// reaches the network on a caller's behalf: an unsynced source is
// pack_unreachable (exit 5) telling the developer to sync.
func TestPackUnreachableWhenSourceNotSynced(t *testing.T) {
	repoDir := stageRepo(t, "one", false)
	_, _, err := Resolve(ResolveInput{
		Home: t.TempDir(), // deliberately empty: nothing is cached
		Cfg:  layeredConfig(0), Team: "Platform",
		Repos: []RepoRef{{Name: "acme.billing.api", LocalPath: repoDir, InstructionsFile: "AGENTS.md"}},
		Now:   fixedClock(beforeWaiverExpiry),
	})
	var envErr *envelope.Error
	if err == nil || !errors.As(err, &envErr) || envErr.Code != "pack_unreachable" {
		t.Fatalf("Resolve error = %v, want code pack_unreachable", err)
	}
	if len(envErr.Details) == 0 || !strings.Contains(envErr.Details[0], "standards sync") {
		t.Errorf("Details = %v, want it to name `standards sync`", envErr.Details)
	}
}

// TestEmbeddedPackIsTheOrgLayerWhenSourcesAreEmpty proves
// config.Validate's own "an empty standards.sources means the embedded
// reference pack" rule holds inside the resolver too.
func TestEmbeddedPackIsTheOrgLayerWhenSourcesAreEmpty(t *testing.T) {
	repoDir := stageRepo(t, "one", false)
	e, packs, err := Resolve(ResolveInput{
		Home: t.TempDir(),
		Cfg:  &config.Config{},
		Repos: []RepoRef{
			{Name: "acme.billing.api", LocalPath: repoDir, InstructionsFile: "AGENTS.md"},
		},
		Languages: []string{"go"},
		Now:       fixedClock(beforeWaiverExpiry),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer ClosePacks(packs)

	if len(e.Packs) == 0 || e.Packs[0].Layer != LayerOrg || e.Packs[0].Ref != EmbeddedRef {
		t.Fatalf("org layer = %+v, want the embedded reference pack", e.Packs[0])
	}
	if _, ok := e.Section("go.errors"); !ok {
		t.Error("the embedded reference pack's go.errors section did not resolve")
	}
}
