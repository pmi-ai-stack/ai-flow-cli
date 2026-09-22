// Package routing implements the spec's documented routing precedence
// (project_key_map → keyword_to_repo → "the ticket names one") as pure
// functions over config.Routing: no I/O beyond stat-ing a repository's
// local checkout for its instructions file. internal/cli's repo.go
// wraps Locate/Confirm with the handoff writes and stage transition.
package routing

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// RepoCandidate is one resolved repository: its name, its local clone
// path (config.routing.teams.<team>.local_base + "/" + name, ~
// expanded) and which instructions file it carries.
type RepoCandidate struct {
	Name             string
	LocalPath        string
	InstructionsFile string
}

// Result is what Locate/Confirm resolve to: repo.json's team and
// repositories, before the CLI layer writes it.
type Result struct {
	Team  string
	Repos []RepoCandidate
}

// ProjectKeyFromTicketKey returns key's project-key prefix: everything
// before the last "-". handoff.KeyPattern (^[A-Z][A-Z0-9]*-[0-9]+$)
// guarantees a "-" exists, so this is never called on a key that
// doesn't have one in production; a key with no "-" returns itself
// unchanged rather than panicking.
func ProjectKeyFromTicketKey(key string) string {
	if idx := strings.LastIndex(key, "-"); idx >= 0 {
		return key[:idx]
	}
	return key
}

// ambiguousError builds the routing_ambiguous error (exit 4):
// candidates, sorted and deduped, ride in Details — internal/cli reads
// them back to build needs_input.
func ambiguousError(candidates []string) *envelope.Error {
	sorted := strutil.DedupSorted(candidates)
	return &envelope.Error{
		Code:    "routing_ambiguous",
		Message: "routing is ambiguous; the developer must choose the repositories",
		Details: sorted,
	}
}

// allConfiguredRepos returns every repository named anywhere in m,
// sorted and deduped — the candidates offered when no project key
// matches at all and there is no narrower set to offer instead.
func allConfiguredRepos(m map[string]config.ProjectKeyMapEntry) []string {
	var all []string
	for _, entry := range m {
		all = append(all, entry.Repos...)
	}
	return strutil.DedupSorted(all)
}

// narrowByKeyword applies keyword_to_repo: candidates whose keyword
// appears (case-insensitively) as a substring of text, intersected
// with candidates; an empty match (no keyword found, or the matched
// repos fall outside candidates) leaves candidates unchanged — "when
// the narrowing is empty every candidate stays in scope". Candidate
// order is preserved.
func narrowByKeyword(kw map[string][]string, candidates []string, text string) []string {
	lower := strings.ToLower(text)
	matched := map[string]bool{}
	anyKeywordFound := false
	for keyword, repos := range kw {
		if keyword == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(keyword)) {
			anyKeywordFound = true
			for _, r := range repos {
				matched[r] = true
			}
		}
	}
	if !anyKeywordFound {
		return slices.Clone(candidates)
	}
	var out []string
	for _, c := range candidates {
		if matched[c] {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return slices.Clone(candidates)
	}
	return out
}

// namedInText returns the subset of candidates whose exact name is a
// case-insensitive substring of text, in candidates' own order — "the
// ticket names X" (design decision 3(e)); a ticket can legitimately
// name more than one repository.
func namedInText(candidates []string, text string) []string {
	lower := strings.ToLower(text)
	var out []string
	for _, c := range candidates {
		if strings.Contains(lower, strings.ToLower(c)) {
			out = append(out, c)
		}
	}
	return out
}

// Locate implements `repo locate KEY`: the spec's routing precedence
// over the ticket key's own project-key prefix and ticketText (the
// fetched ticket's summary+description). Returns routing_ambiguous
// (exit 4, candidates in the error's Details) exactly at the two
// documented ambiguity points; missing_repo_instructions (exit 4, no
// candidates to offer) when a resolved repository's local checkout has
// neither AGENTS.md nor CLAUDE.md.
func Locate(cfg config.Routing, key, ticketText string) (Result, error) {
	projectKey := ProjectKeyFromTicketKey(key)
	entry, ok := cfg.ProjectKeyMap[projectKey]
	if !ok {
		return Result{}, ambiguousError(allConfiguredRepos(cfg.ProjectKeyMap))
	}

	candidates := slices.Clone(entry.Repos)
	narrowed := narrowByKeyword(cfg.KeywordToRepo, candidates, ticketText)

	var resolved []string
	switch {
	case len(narrowed) <= 1:
		resolved = narrowed
	default:
		named := namedInText(narrowed, ticketText)
		if len(named) == 0 {
			return Result{}, ambiguousError(narrowed)
		}
		resolved = named
	}

	return buildResult(cfg, entry.Team, resolved)
}

// reverseTeamIndex maps every repository named anywhere in m to its
// team. Iteration is over sorted project keys so a repository listed
// (inconsistently) under more than one team resolves deterministically
// to the first team in project-key order, not to whichever map
// iteration happened to visit last.
func reverseTeamIndex(m map[string]config.ProjectKeyMapEntry) map[string]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	idx := map[string]string{}
	for _, pk := range keys {
		for _, repo := range m[pk].Repos {
			if _, exists := idx[repo]; !exists {
				idx[repo] = m[pk].Team
			}
		}
	}
	return idx
}

// Confirm implements `repo confirm KEY --repos R...`: the developer
// names the repositories directly, and the team is derived by reverse
// lookup through project_key_map (design decision 4) — every named
// repository must belong to the same team, and every named repository
// must be listed somewhere in project_key_map, else usage (exit 2): a
// human-named repository ai-flow does not know about, or a set
// spanning more than one team, is a config/caller mistake to fix, not
// an ambiguity to ask about again.
func Confirm(cfg config.Routing, repoNames []string) (Result, error) {
	if len(repoNames) == 0 {
		return Result{}, &envelope.Error{Code: "usage", Message: "--repos requires at least one repository name"}
	}
	reverse := reverseTeamIndex(cfg.ProjectKeyMap)

	var team string
	for i, name := range repoNames {
		t, ok := reverse[name]
		if !ok {
			return Result{}, &envelope.Error{
				Code:    "usage",
				Message: fmt.Sprintf("repo %q is not listed in any project_key_map entry", name),
			}
		}
		if i == 0 {
			team = t
			continue
		}
		if t != team {
			return Result{}, &envelope.Error{
				Code:    "usage",
				Message: fmt.Sprintf("repo %q belongs to team %q, not %q — --repos must name repositories of one team", name, t, team),
			}
		}
	}

	return buildResult(cfg, team, repoNames)
}

// buildResult resolves each of repoNames against team's local_base:
// the local checkout path and its instructions file. team must exist
// in cfg.Teams (config validate already enforces this for every
// project_key_map team; Confirm derives team from project_key_map
// itself, so the invariant holds there too).
func buildResult(cfg config.Routing, team string, repoNames []string) (Result, error) {
	t, ok := cfg.Teams[team]
	if !ok {
		return Result{}, fmt.Errorf("routing: team %q not found in config.routing.teams", team)
	}
	base, err := expandHome(t.LocalBase)
	if err != nil {
		return Result{}, fmt.Errorf("routing: expand local_base %q: %w", t.LocalBase, err)
	}

	repos := make([]RepoCandidate, 0, len(repoNames))
	for _, name := range repoNames {
		localPath := filepath.Join(base, name)
		instructionsFile, err := instructionsFileFor(localPath)
		if err != nil {
			return Result{}, err
		}
		repos = append(repos, RepoCandidate{Name: name, LocalPath: localPath, InstructionsFile: instructionsFile})
	}
	return Result{Team: team, Repos: repos}, nil
}

// instructionsFileFor reports which instructions file repoPath's local
// checkout carries: AGENTS.md, else CLAUDE.md, else
// missing_repo_instructions (exit 4, no needs_input — the developer
// must add the file, there is nothing to ask).
func instructionsFileFor(repoPath string) (string, error) {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(repoPath, name)); err == nil {
			return name, nil
		}
	}
	return "", &envelope.Error{
		Code:    "missing_repo_instructions",
		Message: fmt.Sprintf("%s has neither AGENTS.md nor CLAUDE.md", repoPath),
	}
}

// expandHome expands a leading "~" (or "~/...") to the current user's
// home directory; any other path is returned unchanged.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}
