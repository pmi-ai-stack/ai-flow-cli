package standards

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// RepoInstructionsSectionID is the one id the spec names for a
// repository's own instructions file, injected verbatim as a `must`
// section into the implement and review stages.
const RepoInstructionsSectionID = "repo.instructions"

// repoInstructionsPackID is the synthetic pack that carries that one
// section. Deliberately a second constant with the same spelling: pack
// ids and section ids are different namespaces, and collapsing them
// onto one name would make a grep for either turn up both.
const repoInstructionsPackID = "repo.instructions"

// repoDotDir is ai-flow's own directory inside a repository checkout —
// the one place this package spells it, so the pack directory and the
// waivers file (waivers.go) cannot drift apart.
const repoDotDir = ".ai-flow"

// RepoPackDir is where a repository's own optional pack lives inside
// its checkout.
const RepoPackDir = repoDotDir + "/standards"

// RepoRef is one in-scope repository, as much of repo.json as the
// resolver needs.
type RepoRef struct {
	Name             string
	LocalPath        string
	InstructionsFile string
}

// LayerInput carries LoadLayers's inputs.
type LayerInput struct {
	// Home is ai-flow's home directory — where a synced pack's cache
	// entry lives.
	Home string
	// Cfg supplies the org layer (config.standards.sources) and the
	// team layer (config.routing.teams.<team>.standards).
	Cfg *config.Config
	// Team is repo.json's own team name, selecting the team layer.
	Team string
	// Repos is every in-scope repository, in repo.json order — the
	// order their instructions files are concatenated in.
	Repos []RepoRef
}

// LoadLayers loads every pack that contributes to a resolution, in
// precedence order: org first (least specific), then team, then repo.
// The caller closes them with ClosePacks.
//
// The org layer is config.standards.sources, or the embedded reference
// pack when that list is empty. The team layer is the named team's own
// sources. The repo layer is each in-scope repository's optional
// <repo>/.ai-flow/standards/ pack, followed by the synthetic
// repo.instructions pack carrying every repository's instructions file
// verbatim.
//
// A source configured but not yet synced is pack_unreachable (exit 5)
// naming `standards sync`; a repository with neither AGENTS.md nor
// CLAUDE.md is missing_repo_instructions (exit 4).
func LoadLayers(in LayerInput) ([]*Pack, error) {
	var out []*Pack
	fail := func(err error) ([]*Pack, error) {
		ClosePacks(out)
		return nil, err
	}

	orgSources := in.Cfg.Standards.Sources
	if len(orgSources) == 0 {
		p, err := EmbeddedPack()
		if err != nil {
			return fail(err)
		}
		out = append(out, p)
	}
	for _, s := range orgSources {
		p, err := openSyncedPack(in.Home, LayerOrg, s)
		if err != nil {
			return fail(err)
		}
		out = append(out, p)
	}

	for _, s := range in.Cfg.Routing.Teams[in.Team].Standards {
		p, err := openSyncedPack(in.Home, LayerTeam, s)
		if err != nil {
			return fail(err)
		}
		out = append(out, p)
	}

	for _, r := range in.Repos {
		dir := filepath.Join(r.LocalPath, RepoPackDir)
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
			continue // a repository need not carry a pack of its own
		}
		// Repo is left empty deliberately: it would otherwise be the
		// repository's absolute local_path, and 07-determinism.md
		// forbids an absolute path inside any artifact ("paths inside
		// artifacts are relative to the repo or the handoff dir"). An
		// in-tree pack has no upstream URL to record anyway — its id
		// already names the repository.
		p, err := OpenPack(LayerRepo, r.Name, "workdir", "", "", dir)
		if err != nil {
			return fail(err)
		}
		out = append(out, p)
	}

	instructions, err := instructionsPack(in.Repos)
	if err != nil {
		return fail(err)
	}
	out = append(out, instructions)

	return out, nil
}

// openSyncedPack opens one configured source from its sync cache
// directory, refusing with pack_unreachable when it has not been
// synced yet — the resolver never syncs on the caller's behalf, since
// a resolve during `stage start` must not silently reach the network.
func openSyncedPack(home, layer string, s config.StandardsSource) (*Pack, error) {
	// One StatusOf call answers both questions this needs — is it
	// cached, and which commit was it synced at — rather than stat-ing
	// the cache directory here and again inside StatusOf.
	st := StatusOf(home, s.ID, s.Ref, nil)
	if !st.Cached {
		return nil, &envelope.Error{
			Code:    "pack_unreachable",
			Message: fmt.Sprintf("%s@%s is not synced", s.ID, s.Ref),
			Details: []string{"run `ai-flow standards sync --source " + s.ID + "`"},
		}
	}
	return OpenPack(layer, s.ID, s.Ref, s.Repo, st.Commit, CacheDir(home, s.ID, s.Ref))
}

// instructionsPack builds the synthetic repo-layer pack carrying the
// one repo.instructions section: every in-scope repository's
// instructions file (AGENTS.md, else CLAUDE.md), verbatim, in
// repo.json order.
//
// One section, not one per repository: the spec names exactly one id,
// and an invented per-repository id would be a shape no skill, lint
// rule or pack manifest could anticipate. With more than one
// repository each file is preceded by an HTML comment naming its
// source — invisible in rendered Markdown, and unable to collide with
// guidance content the way a `##` heading could. With exactly one
// repository the section is the file's bytes and nothing else.
func instructionsPack(repos []RepoRef) (*Pack, error) {
	var body []byte
	var missing []string
	var files []string

	for _, r := range repos {
		name := r.InstructionsFile
		if name == "" {
			name = "AGENTS.md"
		}
		path := filepath.Join(r.LocalPath, name)
		data, err := os.ReadFile(path) //nolint:gosec // G304: path is built from repo.json's own local_path, explicit CLI input
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("standards: read %s: %w", path, err)
			}
			missing = append(missing, fmt.Sprintf("%s has neither AGENTS.md nor CLAUDE.md", r.Name))
			continue
		}
		files = append(files, name)
		if len(repos) > 1 {
			body = append(body, []byte(fmt.Sprintf("<!-- %s: %s -->\n\n", r.Name, name))...)
		}
		body = append(body, data...)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			body = append(body, '\n')
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, &envelope.Error{
			Code:    "missing_repo_instructions",
			Message: "every in-scope repository must carry an AGENTS.md or a CLAUDE.md",
			Details: missing,
		}
	}

	path := strings.Join(strutil.DedupSorted(files), ", ")
	sum := sha256.Sum256(body)
	return &Pack{
		ID:          repoInstructionsPackID,
		Layer:       LayerRepo,
		Ref:         "workdir",
		Fingerprint: sha256Prefixed(sum[:]),
		Sections: []Section{{
			ID:   RepoInstructionsSectionID,
			Path: path,
			// implement and review only, not every stage: the
			// instructions describe how to write code in this
			// repository, which is what implement does and what review
			// judges.
			Severity:  "must",
			AppliesTo: AppliesTo{Stages: []string{"implement", "review"}},
		}},
		FS: oneFileFS{name: path, data: body},
	}, nil
}
