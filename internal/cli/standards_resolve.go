package cli

import (
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// resolveForTicket performs one standards resolution for a ticket:
// read repo.json and ops-targets.json, then resolveRepos.
//
// Shared verbatim by `standards resolve`, `standards show`, `standards
// waive`, `standards check KEY` and `stage start`, so none of them can
// disagree about what a ticket's effective standards are.
//
// The caller closes the returned packs with standards.ClosePacks once
// it has finished reading section content — the Effective borrows them.
func resolveForTicket(f Flags, key string, cfg *config.Config) (*standards.Effective, []*standards.Pack, error) {
	ticketDir := handoff.TicketDir(f.Home, key)
	artifact, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return nil, nil, err
	}

	// Addons: every addon ops-targets.json names, repo-wide rather than
	// diff-scoped (the same symmetry Languages already has via
	// detect.Languages, not the diff-only ops.Target.Changed) — so a
	// section/check narrowed by applies_to.addons is part of the
	// resolution for the whole ticket, the same way a language check
	// is, not just once a target happens to be the one currently
	// changed. Tolerant of ops-targets.json not existing yet (fetch,
	// locate, or a ticket with no OPS content at all): nil Addons is
	// the correct answer then, not an error.
	opsArtifact, err := loadOpsTargetsTolerant(ticketDir)
	if err != nil {
		return nil, nil, err
	}
	var targets []ops.Target
	if opsArtifact != nil {
		targets = opsArtifact.Targets
	}

	return resolveRepos(f, cfg, resolveScope{
		Team: artifact.Team, Repos: artifact.Repositories, Addons: addonsOf(targets),
	})
}

// resolveScope is what one resolution is over: the repositories, the
// team naming the team layer, and the addons detected in them — plus
// the two options only `standards check <dir>` sets.
type resolveScope struct {
	Team   string
	Repos  []repoArtifactRepository
	Addons []string

	OrgPack              *standards.PackLocation
	InstructionsOptional bool
}

// resolveRepos is the one place a set of repositories becomes a
// resolution: languages detected per repository, waivers loaded per
// repository, then standards.Resolve. The ticket form (resolveForTicket)
// and the directory form (runStandardsCheckDir) both build their
// resolution here, so neither can again drop an input the other sets —
// the directory form once resolved with Languages only, and so never
// ran a single addon-scoped check.
func resolveRepos(f Flags, cfg *config.Config, s resolveScope) (*standards.Effective, []*standards.Pack, error) {
	repos := make([]standards.RepoRef, 0, len(s.Repos))
	var languages []string
	for _, r := range s.Repos {
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

	// Languages and Addons are not deduped here: Resolve normalizes
	// both as part of its own contract.
	return standards.Resolve(standards.ResolveInput{
		Home:                 f.Home,
		Cfg:                  cfg,
		Team:                 s.Team,
		Repos:                repos,
		Languages:            languages,
		Addons:               s.Addons,
		Waivers:              waivers,
		BudgetTokens:         cfg.Standards.GuidanceBudgetTokens,
		OrgPack:              s.OrgPack,
		InstructionsOptional: s.InstructionsOptional,
	})
}

// addonsOf is every target's addon, in target order.
func addonsOf(targets []ops.Target) []string {
	addons := make([]string, 0, len(targets))
	for _, t := range targets {
		addons = append(addons, t.Addon)
	}
	return addons
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
