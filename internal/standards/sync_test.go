package standards

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// fileURL is dir as a file URL. On Windows dir is C:\..., and
// "file://"+dir does not parse (the drive colon reads as a port): the
// URL form is file:///C:/..., which go-git's file transport turns back
// into C:/... before running git-upload-pack.
func fileURL(dir string) string {
	return (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")}).String()
}

// fixtureRemote is a non-bare local git repository this test drives
// entirely through go-git — no os/exec, matching the phase-12 plan's
// design decision 3. It carries two commits: the first tagged v1.0.0,
// the second (HEAD of the default branch) so a tag sync and a branch
// sync land on genuinely different commits.
type fixtureRemote struct {
	URL        string // file:// — forces Sync past isLocalPath's plain-directory shortcut
	Tag        string
	TaggedHash string
	HeadHash   string
	Branch     string
}

func newFixtureRemote(t *testing.T) fixtureRemote {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	writeFile(t, filepath.Join(dir, "standards.json"), fixtureManifestJSON)
	writeFile(t, filepath.Join(dir, "org", "guidance", "x.md"), "## X\n\n1. First.\n")
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "fixture", Email: "fixture@example.com", When: time.Unix(1700000000, 0).UTC()}
	taggedHash, err := wt.Commit("first", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("Commit (first): %v", err)
	}
	if _, err := repo.CreateTag("v1.0.0", taggedHash, nil); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	writeFile(t, filepath.Join(dir, "org", "guidance", "y.md"), "## Y\n\n1. Second.\n")
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	headHash, err := wt.Commit("second", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("Commit (second): %v", err)
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	return fixtureRemote{
		URL:        fileURL(dir),
		Tag:        "v1.0.0",
		TaggedHash: taggedHash.String(),
		HeadHash:   headHash.String(),
		Branch:     head.Name().Short(),
	}
}

const fixtureManifestJSON = `{
  "schema_version": 1, "id": "fixture", "version": "1.0.0",
  "sections": [], "checks": []
}`

func TestSyncByTag(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()

	synced, err := Sync(context.Background(), SyncOptions{
		Home:    home,
		Sources: []Source{{ID: "fixture", Repo: remote.URL, Ref: remote.Tag, Layer: "org"}},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(synced) != 1 || synced[0].RefKind != RefKindTag {
		t.Fatalf("synced = %+v, want one RefKindTag entry", synced)
	}
	assertFileContains(t, filepath.Join(synced[0].Path, "org", "guidance", "x.md"), "First")
	assertNotExists(t, filepath.Join(synced[0].Path, "org", "guidance", "y.md")) // tag predates the second commit
	assertNotExists(t, filepath.Join(synced[0].Path, ".git"))
}

func TestSyncByCommit(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()

	synced, err := Sync(context.Background(), SyncOptions{
		Home:    home,
		Sources: []Source{{ID: "fixture", Repo: remote.URL, Ref: remote.TaggedHash, Layer: "org"}},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(synced) != 1 || synced[0].RefKind != RefKindCommit {
		t.Fatalf("synced = %+v, want one RefKindCommit entry", synced)
	}
	assertFileContains(t, filepath.Join(synced[0].Path, "org", "guidance", "x.md"), "First")
	assertNotExists(t, filepath.Join(synced[0].Path, "org", "guidance", "y.md"))
}

func TestSyncByBranch(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()

	synced, err := Sync(context.Background(), SyncOptions{
		Home:    home,
		Sources: []Source{{ID: "fixture", Repo: remote.URL, Ref: remote.Branch, Layer: "org"}},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(synced) != 1 || synced[0].RefKind != RefKindBranch {
		t.Fatalf("synced = %+v, want one RefKindBranch entry", synced)
	}
	assertFileContains(t, filepath.Join(synced[0].Path, "org", "guidance", "y.md"), "Second")
}

// TestSyncTagCommitAndLocalPathProduceIdenticalTrees is the phase's own
// "Tests" line: sync from a tag, a commit and a local path produce
// identical trees.
func TestSyncTagCommitAndLocalPathProduceIdenticalTrees(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()

	byTag, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{{ID: "a", Repo: remote.URL, Ref: remote.Tag}}})
	if err != nil {
		t.Fatalf("Sync by tag: %v", err)
	}
	byCommit, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{{ID: "b", Repo: remote.URL, Ref: remote.TaggedHash}}})
	if err != nil {
		t.Fatalf("Sync by commit: %v", err)
	}

	localSrc := t.TempDir()
	writeFile(t, filepath.Join(localSrc, "standards.json"), fixtureManifestJSON)
	writeFile(t, filepath.Join(localSrc, "org", "guidance", "x.md"), "## X\n\n1. First.\n")
	byLocal, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{{ID: "c", Repo: localSrc, Ref: "local"}}})
	if err != nil {
		t.Fatalf("Sync by local path: %v", err)
	}

	tagTree := treeFingerprint(t, byTag[0].Path)
	commitTree := treeFingerprint(t, byCommit[0].Path)
	localTree := treeFingerprint(t, byLocal[0].Path)
	if tagTree != commitTree {
		t.Errorf("tag tree %v != commit tree %v", tagTree, commitTree)
	}
	if tagTree != localTree {
		t.Errorf("tag tree %v != local-path tree %v", tagTree, localTree)
	}
}

func TestSyncSkipsAlreadyCachedPinnedRef(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()
	src := Source{ID: "fixture", Repo: remote.URL, Ref: remote.Tag}

	first, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{src}})
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if first[0].Skipped {
		t.Fatal("first sync reported Skipped, want a real fetch")
	}

	second, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{src}})
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if !second[0].Skipped {
		t.Error("second sync of the same pinned tag did not report Skipped")
	}
}

func TestSyncBranchReSyncBoundary(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()
	src := Source{ID: "fixture", Repo: remote.URL, Ref: remote.Branch}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	clock := func() time.Time { return now }

	if _, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{src}, Now: clock}); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	now = base.Add(23 * time.Hour)
	within, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{src}, Now: clock})
	if err != nil {
		t.Fatalf("Sync within 24h: %v", err)
	}
	if !within[0].Skipped {
		t.Error("branch re-sync within 24h should be Skipped")
	}

	now = base.Add(25 * time.Hour)
	after, err := Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{src}, Now: clock})
	if err != nil {
		t.Fatalf("Sync after 24h: %v", err)
	}
	if after[0].Skipped {
		t.Error("branch re-sync after 24h should not be Skipped")
	}
}

func TestSyncOnlyFilter(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()

	synced, err := Sync(context.Background(), SyncOptions{
		Home: home,
		Sources: []Source{
			{ID: "a", Repo: remote.URL, Ref: remote.Tag},
			{ID: "b", Repo: remote.URL, Ref: remote.Tag},
		},
		Only: "b",
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(synced) != 1 || synced[0].ID != "b" {
		t.Errorf("synced = %+v, want exactly source \"b\"", synced)
	}
}

func TestSyncUnknownRefIsPackUnreachable(t *testing.T) {
	remote := newFixtureRemote(t)
	home := t.TempDir()

	_, err := Sync(context.Background(), SyncOptions{
		Home:    home,
		Sources: []Source{{ID: "fixture", Repo: remote.URL, Ref: "no-such-ref"}},
	})
	if err == nil {
		t.Fatal("Sync(unknown ref) = nil error, want pack_unreachable")
	}
}

func TestSourcesFromConfigOrdersTeamsByName(t *testing.T) {
	cfg := configWithSources()
	sources := SourcesFromConfig(cfg)
	if len(sources) != 3 {
		t.Fatalf("len(sources) = %d, want 3", len(sources))
	}
	if sources[0].Layer != "org" || sources[0].ID != "acme-org" {
		t.Errorf("sources[0] = %+v, want the org source first", sources[0])
	}
	if sources[1].ID != "alpha-team-pack" || sources[2].ID != "beta-team-pack" {
		t.Errorf("team sources not ordered by team name: %+v", sources[1:])
	}
}

func TestGitHubTokenFromEnvFallbackOrder(t *testing.T) {
	t.Setenv("AI_FLOW_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	if got := GitHubTokenFromEnv(); got != "" {
		t.Errorf("GitHubTokenFromEnv() = %q, want empty", got)
	}

	t.Setenv("GH_TOKEN", "gh-token")
	if got := GitHubTokenFromEnv(); got != "gh-token" {
		t.Errorf("GitHubTokenFromEnv() = %q, want gh-token", got)
	}

	t.Setenv("GITHUB_TOKEN", "github-token")
	if got := GitHubTokenFromEnv(); got != "github-token" {
		t.Errorf("GitHubTokenFromEnv() = %q, want github-token (beats GH_TOKEN)", got)
	}

	t.Setenv("AI_FLOW_GITHUB_TOKEN", "ai-flow-token")
	if got := GitHubTokenFromEnv(); got != "ai-flow-token" {
		t.Errorf("GitHubTokenFromEnv() = %q, want ai-flow-token (beats both fallbacks)", got)
	}
}

// TestAuthForOnlyOffersCredentialToGitHubOrOptIn is SEC-13: the GitHub
// token goes to github.com over https, or to another https host only
// when that source says `auth: true` — never to whatever host a pack
// URL happens to name.
func TestAuthForOnlyOffersCredentialToGitHubOrOptIn(t *testing.T) {
	cases := []struct {
		url   string
		token string
		optIn bool
		want  bool
	}{
		{"https://github.com/acme/standards.git", "tok", false, true},
		{"https://GitHub.com/acme/standards.git", "tok", false, true},
		{"https://github.com/acme/standards.git", "", false, false},
		{"https://example.com/pack.git", "tok", false, false},
		{"https://github.com.attacker.example/pack.git", "tok", false, false},
		{"https://ghe.acme.internal/org/pack.git", "tok", true, true},
		{"http://github.com/acme/standards.git", "tok", true, false},
		{"git@github.com:org/pack.git", "tok", true, false},
	}
	for _, c := range cases {
		if got := authFor(c.url, c.token, c.optIn) != nil; got != c.want {
			t.Errorf("authFor(%q, token=%v, optIn=%v) offered = %v, want %v", c.url, c.token != "", c.optIn, got, c.want)
		}
	}
}

func TestSourcesFromConfigCarriesAuth(t *testing.T) {
	cfg := &config.Config{Standards: config.Standards{Sources: []config.StandardsSource{
		{ID: "ghe", Repo: "https://ghe.acme.internal/org/pack.git", Ref: "v1", Auth: true},
	}}}
	if got := SourcesFromConfig(cfg); len(got) != 1 || !got[0].Auth {
		t.Errorf("SourcesFromConfig = %+v, want Auth carried through", got)
	}
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), substr) {
		t.Errorf("%s does not contain %q:\n%s", path, substr, data)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s exists, want it absent", path)
	}
}

// treeFingerprint hashes every regular file's relative path and
// content under dir into one deterministic string — enough to assert
// "these two synced trees are identical" without a heavier content-
// addressable tree implementation.
func treeFingerprint(t *testing.T, dir string) string {
	t.Helper()
	var out string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out += rel + ":" + string(data) + "\n"
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}
