package cli

import (
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// resolveForTicket performs one standards resolution for a ticket:
// read repo.json, detect the languages across every in-scope
// repository, load every repository's waivers, and resolve.
//
// Shared verbatim by `standards resolve`, `standards show`, `standards
// waive` and `stage start`, so all four can never disagree about what
// a ticket's effective standards are.
//
// The caller closes the returned packs with standards.ClosePacks once
// it has finished reading section content — the Effective borrows them.
func resolveForTicket(f Flags, key string, cfg *config.Config) (*standards.Effective, []*standards.Pack, error) {
	ticketDir := handoff.TicketDir(f.Home, key)
	artifact, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return nil, nil, err
	}

	repos := make([]standards.RepoRef, 0, len(artifact.Repositories))
	var languages []string
	for _, r := range artifact.Repositories {
		repos = append(repos, standards.RepoRef{
			Name: r.Name, LocalPath: r.LocalPath, InstructionsFile: r.InstructionsFile,
		})
		langs, err := detect.Languages(toDetectRepo(r))
		if err != nil {
			return nil, nil, fmt.Errorf("cli: detect languages in %s: %w", r.Name, err)
		}
		languages = append(languages, langs...)
	}

	var waivers []standards.Waiver
	for _, r := range repos {
		w, err := standards.LoadWaivers(r.Name, r.LocalPath)
		if err != nil {
			return nil, nil, err
		}
		waivers = append(waivers, w...)
	}

	return standards.Resolve(standards.ResolveInput{
		Home:  f.Home,
		Cfg:   cfg,
		Team:  artifact.Team,
		Repos: repos,
		// Not deduped here: Resolve normalizes Languages and Addons as
		// part of its own contract, since they are resolution-hash
		// inputs and it cannot trust a caller to have done it.
		Languages: languages,
		// Addons stays empty until phase 18's `ops detect` exists. Not a
		// stub: no addon is active, so no addon-narrowed entry applies,
		// which is the correct answer today rather than a placeholder.
		Addons:       nil,
		Waivers:      waivers,
		BudgetTokens: cfg.Standards.GuidanceBudgetTokens,
	})
}

// resolveAndWrite resolves for a ticket and writes standards.json when
// the resolution hash moved — `standards resolve`'s own idempotency
// rule, and the refresh rule `stage start` reuses. Returns the
// resolution, the packs, and whether anything was written.
//
// The caller always owns the packs, on every return path including the
// error ones, so the call site is one unconditional
// `defer standards.ClosePacks(packs)` placed before the error check —
// ClosePacks tolerates a nil slice. Closing them here on some paths and
// not others would make that impossible to get right.
func resolveAndWrite(f Flags, key string, cfg *config.Config) (*standards.Effective, []*standards.Pack, bool, error) {
	e, packs, err := resolveForTicket(f, key, cfg)
	if err != nil {
		return nil, packs, false, err
	}

	ticketDir := handoff.TicketDir(f.Home, key)
	previous, err := standards.ReadArtifactHash(ticketDir)
	if err != nil {
		return e, packs, false, err
	}
	if previous == e.Hash {
		return e, packs, false, nil
	}

	if _, err := standards.WriteArtifact(ticketDir, e); err != nil {
		return e, packs, false, err
	}
	return e, packs, true, nil
}
