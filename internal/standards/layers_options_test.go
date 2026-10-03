package standards

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestInstructionsOptionalReportsInsteadOfFailing is the directory
// form's own resolution: a repository without AGENTS.md/CLAUDE.md is
// named in MissingInstructions, no repo.instructions section exists,
// and the rest of the resolution is intact.
func TestInstructionsOptionalReportsInsteadOfFailing(t *testing.T) {
	home := stageHome(t)
	none := stageRepo(t, "none", false)

	e, packs, err := Resolve(ResolveInput{
		Home: home, Cfg: layeredConfig(0), Team: "Platform",
		Repos:                []RepoRef{{Name: "acme.orphan", LocalPath: none, InstructionsFile: "AGENTS.md"}},
		Now:                  fixedClock(beforeWaiverExpiry),
		InstructionsOptional: true,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	t.Cleanup(func() { ClosePacks(packs) })

	if len(e.MissingInstructions) != 1 || e.MissingInstructions[0] != "acme.orphan has neither AGENTS.md nor CLAUDE.md" {
		t.Errorf("MissingInstructions = %v, want the one repository named", e.MissingInstructions)
	}
	for _, s := range e.Sections {
		if s.ID == RepoInstructionsSectionID {
			t.Errorf("repo.instructions section present with no instructions file: %+v", s)
		}
	}
	if len(e.Sections) == 0 {
		t.Error("no sections at all: the org/team layers should still resolve")
	}
}

// TestInstructionsOptionalLeavesPresentFilesUntouched proves the flag
// changes nothing when the file exists: same hash, same sections, no
// MissingInstructions — so a ticket-form resolution can never be
// affected by the directory form's option.
func TestInstructionsOptionalLeavesPresentFilesUntouched(t *testing.T) {
	home := stageHome(t)
	repo := stageRepo(t, "one", false)
	in := ResolveInput{
		Home: home, Cfg: layeredConfig(0), Team: "Platform",
		Repos: []RepoRef{{Name: "acme.billing.api", LocalPath: repo, InstructionsFile: "AGENTS.md"}},
		Now:   fixedClock(beforeWaiverExpiry),
	}
	strict, packsA, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve strict: %v", err)
	}
	t.Cleanup(func() { ClosePacks(packsA) })

	in.InstructionsOptional = true
	lenient, packsB, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve lenient: %v", err)
	}
	t.Cleanup(func() { ClosePacks(packsB) })

	if strict.Hash != lenient.Hash {
		t.Errorf("hash moved with InstructionsOptional and the file present: %s vs %s", strict.Hash, lenient.Hash)
	}
	if lenient.MissingInstructions != nil {
		t.Errorf("MissingInstructions = %v, want nil", lenient.MissingInstructions)
	}
}

// TestOrgPackReplacesConfiguredOrgLayer proves --pack's resolution: the
// given directory is the org layer, configured org sources are not
// consulted (none is synced in this home, so consulting one would be
// pack_unreachable), and the team layer is unaffected.
func TestOrgPackReplacesConfiguredOrgLayer(t *testing.T) {
	home := stageHome(t)
	repo := stageRepo(t, "one", false)
	cfg := layeredConfig(0)
	cfg.Standards.Sources[0].Ref = "never-synced"

	e, packs, err := Resolve(ResolveInput{
		Home: home, Cfg: cfg, Team: "Platform",
		Repos: []RepoRef{{Name: "acme.billing.api", LocalPath: repo, InstructionsFile: "AGENTS.md"}},
		Now:   fixedClock(beforeWaiverExpiry),
		OrgPack: &PackLocation{
			ID: "cli", Ref: "workdir", Dir: filepath.Join("testdata", "layers", "org"),
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	t.Cleanup(func() { ClosePacks(packs) })

	var ids []string
	for _, p := range e.Packs {
		ids = append(ids, p.Layer+":"+p.ID)
	}
	want := []string{LayerOrg + ":cli", LayerTeam + ":platform-team", LayerRepo + ":repo.instructions"}
	if !slices.Equal(ids, want) {
		t.Errorf("packs = %v, want %v", ids, want)
	}
}
