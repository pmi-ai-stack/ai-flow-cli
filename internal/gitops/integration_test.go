//go:build integration

package gitops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// fixtureRows is a test-only allowlist — broader than the production
// Table — used solely to build the bare "origin" repos and working
// clones these tests exercise the real GitOps against. It never
// touches production code; it exists only so fixture setup can go
// through internal/ops/exec's Runner too (TestNoOsExecOutsideExecPackage
// forbids os/exec anywhere else in the module, test files included).
var fixtureRows = []exec.Row{
	{Tool: "git", Verb: []string{"init"}, Mutating: true, Class: exec.ClassRepoWrite},
	{Tool: "git", Verb: []string{"config"}, Mutating: true, Class: exec.ClassRepoWrite},
	{Tool: "git", Verb: []string{"remote"}, Mutating: true, Class: exec.ClassRepoWrite},
	{Tool: "git", Verb: []string{"branch"}, Mutating: true, Class: exec.ClassRepoWrite},
	{Tool: "git", Verb: []string{"log"}, Class: exec.ClassRepoWrite},
}

func fixtureRunner() *exec.Runner {
	return &exec.Runner{Allowlist: exec.NewAllowlist(fixtureRows...), Redact: redact.New(nil)}
}

func mustRun(t *testing.T, r exec.Execer, dir string, args ...string) exec.Output {
	t.Helper()
	out, err := r.Run(context.Background(), "git", args, exec.RunOpts{Dir: dir})
	if err != nil {
		t.Fatalf("git %v in %s: %v (stderr: %s)", args, dir, err, out.Stderr)
	}
	return out
}

// newFixtureRepo builds a bare "origin" repo and a working clone with
// one commit on main pushed to it, using both fixtureRunner (init,
// config, remote — not real production verbs) and the real
// exec.NewRunner() (add, commit, checkout, push — proving the
// production allowlist actually covers what Prepare/Commit/Push need).
func newFixtureRepo(t *testing.T) (origin, work string) {
	t.Helper()
	fr := fixtureRunner()
	real := exec.NewRunner()
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	work = filepath.Join(root, "work")

	mustRun(t, fr, root, "init", "--bare", origin)
	mustRun(t, fr, root, "init", work)
	mustRun(t, fr, work, "config", "user.email", "test@example.com")
	mustRun(t, fr, work, "config", "user.name", "Test")
	mustRun(t, real, work, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustRun(t, real, work, "add", "-A")
	mustRun(t, real, work, "commit", "-m", "initial commit")
	mustRun(t, fr, work, "remote", "add", "origin", origin)
	mustRun(t, real, work, "push", "-u", "origin", "main")
	return origin, work
}

func TestIntegrationPrepareCleanRepoPlainCheckout(t *testing.T) {
	_, work := newFixtureRepo(t)
	home := t.TempDir()
	real := exec.NewRunner()
	g := New(real, 0)

	res, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: work, Base: "main", Branch: "feature/PROJ-1", Home: home, Key: "PROJ-1", RepoName: "acme.api",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.WorkingPath != work {
		t.Errorf("WorkingPath = %q, want %q (plain checkout)", res.WorkingPath, work)
	}
	if !res.BranchCreated {
		t.Error("BranchCreated = false, want true")
	}

	branch := strings.TrimSpace(mustRun(t, real, work, "rev-parse", "--abbrev-ref", "HEAD").Stdout)
	if branch != "feature/PROJ-1" {
		t.Errorf("HEAD branch = %q, want feature/PROJ-1", branch)
	}

	baseSHA := strings.TrimSpace(mustRun(t, real, work, "rev-parse", "origin/main").Stdout)
	if _, err := os.Stat(res.BaseWorktreePath); err != nil {
		t.Fatalf("base worktree %s does not exist: %v", res.BaseWorktreePath, err)
	}
	wtSHA := strings.TrimSpace(mustRun(t, real, res.BaseWorktreePath, "rev-parse", "HEAD").Stdout)
	if wtSHA != baseSHA {
		t.Errorf("base worktree HEAD = %s, want %s (origin/main)", wtSHA, baseSHA)
	}

	// A second Prepare is idempotent: no error, branch not re-created.
	res2, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: work, Base: "main", Branch: "feature/PROJ-1", Home: home, Key: "PROJ-1", RepoName: "acme.api",
	})
	if err != nil {
		t.Fatalf("second Prepare: %v", err)
	}
	if res2.BranchCreated {
		t.Error("second Prepare: BranchCreated = true, want false (already existed)")
	}
}

func TestIntegrationPrepareWorktreeMode(t *testing.T) {
	_, work := newFixtureRepo(t)
	home := t.TempDir()
	g := New(exec.NewRunner(), 0)

	res, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: work, Base: "main", Branch: "feature/PROJ-2", Worktree: true,
		Home: home, Key: "PROJ-2", RepoName: "acme.api",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	wantWT := filepath.Join(filepath.Dir(work), "acme.api-PROJ-2")
	if res.WorkingPath != wantWT {
		t.Errorf("WorkingPath = %q, want %q", res.WorkingPath, wantWT)
	}
	if _, err := os.Stat(filepath.Join(res.WorkingPath, "README.md")); err != nil {
		t.Errorf("worktree %s does not contain README.md: %v", res.WorkingPath, err)
	}
	// The original clone's own branch is untouched.
	branch := strings.TrimSpace(mustRun(t, exec.NewRunner(), work, "rev-parse", "--abbrev-ref", "HEAD").Stdout)
	if branch != "main" {
		t.Errorf("original clone's HEAD branch = %q, want main (untouched)", branch)
	}
}

func TestIntegrationPrepareRefusesDirtyTree(t *testing.T) {
	_, work := newFixtureRepo(t)
	if err := os.WriteFile(filepath.Join(work, "untracked.txt"), []byte("oops\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	g := New(exec.NewRunner(), 0)

	_, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: work, Base: "main", Branch: "feature/PROJ-3", Home: t.TempDir(), Key: "PROJ-3", RepoName: "acme.api",
	})
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("err = %v, want dirty_worktree", err)
	}
}

func TestIntegrationPrepareRefusesDivergedBase(t *testing.T) {
	_, work := newFixtureRepo(t)
	real := exec.NewRunner()
	// Make a local commit on main that is never pushed — main is now
	// ahead of origin/main.
	if err := os.WriteFile(filepath.Join(work, "local-only.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustRun(t, real, work, "add", "-A")
	mustRun(t, real, work, "commit", "-m", "local commit never pushed")

	g := New(real, 0)
	_, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: work, Base: "main", Branch: "feature/PROJ-4", Home: t.TempDir(), Key: "PROJ-4", RepoName: "acme.api",
	})
	if err == nil || !strings.Contains(err.Error(), "has commits not on origin") {
		t.Fatalf("err = %v, want base_diverged", err)
	}
}

func TestIntegrationDiffCommitPushRoundtrip(t *testing.T) {
	origin, work := newFixtureRepo(t)
	home := t.TempDir()
	g := New(exec.NewRunner(), 0)

	if _, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: work, Base: "main", Branch: "feature/PROJ-5", Home: home, Key: "PROJ-5", RepoName: "acme.api",
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// No changes yet: Diff is empty, Commit is a no-op.
	diff, err := g.Diff(context.Background(), work, "main")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diff.Files) != 0 {
		t.Errorf("Files = %v, want empty before any change", diff.Files)
	}
	if committed, err := g.Commit(context.Background(), work, "no-op"); err != nil || committed {
		t.Errorf("Commit on a clean tree: committed=%v err=%v, want false/nil", committed, err)
	}

	if err := os.WriteFile(filepath.Join(work, "new-file.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	diff, err = g.Diff(context.Background(), work, "main")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diff.Files) != 1 || diff.Files[0] != "new-file.txt" {
		t.Errorf("Files = %v, want [new-file.txt]", diff.Files)
	}

	committed, err := g.Commit(context.Background(), work, "add new-file.txt")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Error("committed = false, want true")
	}

	if err := g.Push(context.Background(), work, "feature/PROJ-5"); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Prove the branch really landed on origin: clone-free check via
	// ls-remote-equivalent — rev-parse against a fresh clone of origin.
	verify := filepath.Join(t.TempDir(), "verify")
	mustRun(t, fixtureRunner(), filepath.Dir(verify), "init", verify)
	mustRun(t, fixtureRunner(), verify, "remote", "add", "origin", origin)
	mustRun(t, exec.NewRunner(), verify, "fetch", "origin", "feature/PROJ-5")
	sha := strings.TrimSpace(mustRun(t, exec.NewRunner(), verify, "rev-parse", "origin/feature/PROJ-5").Stdout)
	if sha == "" {
		t.Error("origin/feature/PROJ-5 not found after push")
	}
}

func TestIntegrationRemoteOwnerRepo(t *testing.T) {
	root := t.TempDir()
	originDir := filepath.Join(root, "acme", "billing-api.git")
	if err := os.MkdirAll(filepath.Dir(originDir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	fr := fixtureRunner()
	mustRun(t, fr, root, "init", "--bare", originDir)
	work := filepath.Join(root, "work")
	mustRun(t, fr, root, "init", work)
	mustRun(t, fr, work, "remote", "add", "origin", originDir)

	g := New(exec.NewRunner(), 0)
	owner, repo, err := g.RemoteOwnerRepo(context.Background(), work)
	if err != nil {
		t.Fatalf("RemoteOwnerRepo: %v", err)
	}
	if owner != "acme" || repo != "billing-api" {
		t.Errorf("got (%q, %q), want (acme, billing-api)", owner, repo)
	}
}
