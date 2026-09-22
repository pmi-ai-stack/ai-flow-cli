package standards

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// RefKind classifies a source's Ref, decided by asking the remote
// rather than guessing from Ref's own shape (see the phase-12 plan's
// design decision 4).
type RefKind string

const (
	RefKindTag    RefKind = "tag"
	RefKindCommit RefKind = "commit"
	RefKindBranch RefKind = "branch"
	RefKindLocal  RefKind = "local"
)

// branchReSyncAfter is how stale a branch ref's cache entry must be
// before Sync re-fetches it — the spec's own "re-synced at most once a
// day."
const branchReSyncAfter = 24 * time.Hour

// commitishPattern matches a Ref shaped like a commit hash — used only
// once classifyRef has already ruled out a tag and a branch of that
// exact name on the remote.
var commitishPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// Source is one pack source to sync: an org-layer config.standards.sources
// entry or a team's own config.routing.teams.<team>.standards entry.
type Source struct {
	ID    string
	Repo  string
	Ref   string
	Layer string // "org" or "team"
}

// SourcesFromConfig collects every org and team pack source from cfg,
// team sources ordered by team name for determinism (map iteration
// order is never relied on — 10-data-types.md). The repo layer is not
// included: it needs a repository checkout to read from, which only
// exists once a ticket has run repo locate/git prepare — phase 13's
// resolver, not this package's concern.
func SourcesFromConfig(cfg *config.Config) []Source {
	out := make([]Source, 0, len(cfg.Standards.Sources))
	for _, s := range cfg.Standards.Sources {
		out = append(out, Source{ID: s.ID, Repo: s.Repo, Ref: s.Ref, Layer: "org"})
	}

	teamNames := make([]string, 0, len(cfg.Routing.Teams))
	for name := range cfg.Routing.Teams {
		teamNames = append(teamNames, name)
	}
	sort.Strings(teamNames)
	for _, name := range teamNames {
		for _, s := range cfg.Routing.Teams[name].Standards {
			out = append(out, Source{ID: s.ID, Repo: s.Repo, Ref: s.Ref, Layer: "team"})
		}
	}
	return out
}

// SyncedSource is one Source's Sync outcome.
type SyncedSource struct {
	ID            string
	Ref           string
	RefKind       RefKind
	Path          string
	Skipped       bool
	SkippedReason string
}

// SyncOptions carries Sync's inputs.
type SyncOptions struct {
	// Home is ai-flow's home directory (--home).
	Home string
	// Sources is every candidate source — org plus every team's own,
	// typically SourcesFromConfig's result.
	Sources []Source
	// Only, when non-empty, restricts Sync to the one source with this
	// ID — `standards sync --source ID`.
	Only string
	// GitHubToken authenticates an https:// source, tried only when the
	// source is not a local filesystem path. Empty means no auth
	// offered (a public source, or one the environment has no
	// credential for).
	GitHubToken string
	// Now stands in for time.Now in tests exercising the 24h branch
	// re-sync boundary; nil means time.Now.
	Now func() time.Time
}

// Sync fetches every matching source into
// CacheDir(opts.Home, source.ID, source.Ref), skipping a pinned
// tag/commit already cached and a branch cached within the last day.
// It stops at the first source it cannot sync — a partial []SyncedSource
// is still returned alongside the error so the caller can report what
// did succeed.
func Sync(opts SyncOptions) ([]SyncedSource, error) {
	now := opts.Now
	if now == nil {
		now = defaultClock
	}

	var results []SyncedSource
	for _, src := range opts.Sources {
		if opts.Only != "" && src.ID != opts.Only {
			continue
		}
		res, err := syncOne(opts.Home, src, opts.GitHubToken, now)
		if err != nil {
			return results, err
		}
		results = append(results, res)
	}
	return results, nil
}

func syncOne(home string, src Source, token string, now func() time.Time) (SyncedSource, error) {
	cacheDir := CacheDir(home, src.ID, src.Ref)
	metaPath := SyncMetaPath(home, src.ID, src.Ref)

	if isLocalPath(src.Repo) {
		if err := copyLocalPack(src.Repo, cacheDir); err != nil {
			return SyncedSource{}, &envelope.Error{Code: "pack_unreachable", Message: fmt.Sprintf("%s: %v", src.ID, err)}
		}
		if err := writeSyncMeta(metaPath, now(), RefKindLocal); err != nil {
			return SyncedSource{}, err
		}
		return SyncedSource{ID: src.ID, Ref: src.Ref, RefKind: RefKindLocal, Path: cacheDir}, nil
	}

	if meta, err := readSyncMeta(metaPath); err == nil && dirExists(cacheDir) {
		switch RefKind(meta.RefKind) {
		case RefKindTag, RefKindCommit:
			return SyncedSource{ID: src.ID, Ref: src.Ref, RefKind: RefKind(meta.RefKind), Path: cacheDir, Skipped: true, SkippedReason: "pinned ref already synced"}, nil
		case RefKindBranch:
			if syncedAt, perr := time.Parse(time.RFC3339, meta.SyncedAt); perr == nil && now().Sub(syncedAt) < branchReSyncAfter {
				return SyncedSource{ID: src.ID, Ref: src.Ref, RefKind: RefKindBranch, Path: cacheDir, Skipped: true, SkippedReason: "branch ref synced within the last day"}, nil
			}
		case RefKindLocal:
			// A source cannot change kind across runs without an id or
			// ref change, since CacheDir/SyncMetaPath are keyed on
			// both — unreachable in practice, handled for completeness.
		}
	}

	auth := authFor(src.Repo, token)
	kind, refName, err := classifyRef(src.Repo, src.Ref, auth)
	if err != nil {
		return SyncedSource{}, &envelope.Error{Code: "pack_unreachable", Message: fmt.Sprintf("%s: %v", src.ID, err)}
	}

	if err := os.RemoveAll(cacheDir); err != nil {
		return SyncedSource{}, fmt.Errorf("standards: remove stale cache %s: %w", cacheDir, err)
	}

	if err := cloneAt(cacheDir, src.Repo, src.Ref, kind, refName, auth); err != nil {
		return SyncedSource{}, &envelope.Error{Code: "pack_unreachable", Message: fmt.Sprintf("%s: %v", src.ID, err)}
	}
	// Strip .git so the cached tree is pack content only — the same
	// tree a local-path sync of identical content would produce (see
	// design decision 5's tree-identity requirement).
	if err := os.RemoveAll(filepath.Join(cacheDir, ".git")); err != nil {
		return SyncedSource{}, fmt.Errorf("standards: remove %s/.git: %w", cacheDir, err)
	}

	if err := writeSyncMeta(metaPath, now(), kind); err != nil {
		return SyncedSource{}, err
	}
	return SyncedSource{ID: src.ID, Ref: src.Ref, RefKind: kind, Path: cacheDir}, nil
}

// classifyRef asks repoURL's remote (a lightweight ls-remote-equivalent
// list, no clone) whether ref names a tag or a branch there; anything
// else shaped like a commit hash is assumed to be one — a full clone
// is the only way to actually confirm a commit exists, so classifyRef
// defers that confirmation to cloneAt itself.
func classifyRef(repoURL, ref string, auth transport.AuthMethod) (RefKind, plumbing.ReferenceName, error) {
	remote := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{Name: "origin", URLs: []string{repoURL}})
	refs, err := remote.List(&git.ListOptions{Auth: auth})
	if err != nil {
		return "", "", fmt.Errorf("list remote refs: %w", err)
	}

	tagName := plumbing.NewTagReferenceName(ref)
	branchName := plumbing.NewBranchReferenceName(ref)
	for _, r := range refs {
		if r.Name() == tagName {
			return RefKindTag, tagName, nil
		}
	}
	for _, r := range refs {
		if r.Name() == branchName {
			return RefKindBranch, branchName, nil
		}
	}
	if commitishPattern.MatchString(ref) {
		return RefKindCommit, "", nil
	}
	return "", "", fmt.Errorf("ref %q is neither a tag nor a branch on %s, and is not shaped like a commit hash", ref, repoURL)
}

// cloneAt materializes ref (already classified as kind) into dir. A
// tag or branch clones shallow and single-branch, straight to that
// ref; a commit clones the default branch in full and checks the
// commit out, since a shallow clone cannot target an arbitrary
// historical commit without server-side support this package does not
// assume.
func cloneAt(dir, repoURL, ref string, kind RefKind, refName plumbing.ReferenceName, auth transport.AuthMethod) error {
	switch kind {
	case RefKindTag, RefKindBranch:
		_, err := git.PlainClone(dir, false, &git.CloneOptions{
			URL:           repoURL,
			Auth:          auth,
			ReferenceName: refName,
			SingleBranch:  true,
			Depth:         1,
			Tags:          git.NoTags,
		})
		return err
	case RefKindCommit:
		repo, err := git.PlainClone(dir, false, &git.CloneOptions{URL: repoURL, Auth: auth})
		if err != nil {
			return err
		}
		wt, err := repo.Worktree()
		if err != nil {
			return err
		}
		return wt.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(ref)})
	case RefKindLocal:
		return fmt.Errorf("standards: cloneAt called with RefKindLocal, which never reaches the network path")
	default:
		return fmt.Errorf("standards: unknown ref kind %q", kind)
	}
}

// authFor returns http.BasicAuth for an https:// source when token is
// set, else nil (go-git treats a nil AuthMethod as "no auth offered").
// A non-https remote (ssh, a bare local path already handled by
// isLocalPath) never receives this token — it has no https endpoint to
// present it to.
func authFor(repoURL, token string) transport.AuthMethod {
	if token == "" || !strings.HasPrefix(repoURL, "https://") {
		return nil
	}
	return &githttp.BasicAuth{Username: "x-access-token", Password: token}
}

// GitHubTokenFromEnv resolves a private pack source's credential the
// same way internal/cli/pr.go's buildHost already does for the GitHub
// host: AI_FLOW_GITHUB_TOKEN, else GITHUB_TOKEN, else GH_TOKEN. No new
// environment variable — the spec's own secrets table already names
// AI_FLOW_GITHUB_TOKEN for "private pack sources."
func GitHubTokenFromEnv() string {
	for _, name := range []string{"AI_FLOW_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

// isLocalPath reports whether repo names an existing local directory
// rather than a remote URL — a URL scheme (scheme://) or an scp-like
// SSH host (user@host:path) is never treated as local, even if a
// same-named directory happens to exist.
func isLocalPath(repo string) bool {
	if strings.Contains(repo, "://") {
		return false
	}
	if strings.Contains(repo, "@") && strings.Contains(repo, ":") {
		return false
	}
	info, err := os.Stat(repo)
	return err == nil && info.IsDir()
}

// copyLocalPack replaces dst with a full copy of src — unconditional,
// since a local path has no ref/staleness concept and no cost
// advantage to skipping it.
func copyLocalPack(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("remove stale cache %s: %w", dst, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return nil
}

type syncMeta struct {
	SyncedAt string `json:"synced_at"`
	RefKind  string `json:"ref_kind"`
}

func writeSyncMeta(path string, at time.Time, kind RefKind) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("standards: create %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(syncMeta{SyncedAt: at.UTC().Format(time.RFC3339), RefKind: string(kind)}, "", "  ")
	if err != nil {
		return fmt.Errorf("standards: marshal sync meta: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("standards: write %s: %w", path, err)
	}
	return nil
}

func readSyncMeta(path string) (*syncMeta, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is always built from --home + a config-declared source id/ref, not attacker-controlled
	if err != nil {
		return nil, err
	}
	var m syncMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Status is one source's read-only sync state — doctor's own view,
// kept in this package so a caller never has to reach into Sync's
// private sidecar-file format to answer "is this cached, and is it
// stale."
type Status struct {
	Cached  bool
	RefKind RefKind
	Stale   bool
}

// StatusOf reports id@ref's current Status under home. now stands in
// for time.Now in tests, exactly like SyncOptions.Now; nil means
// time.Now.
func StatusOf(home, id, ref string, now func() time.Time) Status {
	if now == nil {
		now = defaultClock
	}
	if !dirExists(CacheDir(home, id, ref)) {
		return Status{}
	}
	meta, err := readSyncMeta(SyncMetaPath(home, id, ref))
	if err != nil {
		// Cached but no readable sync meta — a source restored from
		// elsewhere, or the sidecar predates this field. Report as
		// cached with an unknown ref kind rather than failing.
		return Status{Cached: true}
	}

	st := Status{Cached: true, RefKind: RefKind(meta.RefKind)}
	if st.RefKind == RefKindBranch {
		syncedAt, perr := time.Parse(time.RFC3339, meta.SyncedAt)
		st.Stale = perr != nil || now().Sub(syncedAt) >= branchReSyncAfter
	}
	return st
}
