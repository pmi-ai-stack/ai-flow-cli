//go:build integration

package cli

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// fixtureRows is a test-only allowlist — broader than the production
// Table — used solely to build the bare "origin" repo and working
// clone this test exercises the real git commit/push path against. It
// never touches production code; it exists only so fixture setup can
// go through internal/ops/exec's Runner too
// (TestNoOsExecOutsideExecPackage forbids os/exec anywhere else in the
// module, test files included) — the same pattern
// internal/gitops/integration_test.go already established.
var fixtureRows = []exec.Row{
	{Tool: "git", Verb: []string{"init"}, Mutating: true, Class: exec.ClassRepoWrite},
	{Tool: "git", Verb: []string{"config"}, Mutating: true, Class: exec.ClassRepoWrite},
	{Tool: "git", Verb: []string{"remote"}, Mutating: true, Class: exec.ClassRepoWrite},
	{Tool: "git", Verb: []string{"log"}, Class: exec.ClassRepoWrite},
}

func fixtureRunner() *exec.Runner {
	return &exec.Runner{Allowlist: exec.NewAllowlist(fixtureRows...), Redact: redact.New(nil)}
}

func mustRunGit(t *testing.T, r exec.Execer, dir string, args ...string) exec.Output {
	t.Helper()
	out, err := r.Run(context.Background(), "git", args, exec.RunOpts{Dir: dir})
	if err != nil {
		t.Fatalf("git %v in %s: %v (stderr: %s)", args, dir, err, out.Stderr)
	}
	return out
}

// newFixtureRepoOnBranch builds a bare "origin" repo and a working
// clone with one commit on main pushed to it, then checks out branch —
// using the real production exec.NewRunner() for every verb the
// production allowlist actually needs to cover (init/config/remote add
// are fixture-only setup, exactly as internal/gitops's own integration
// suite already established).
func newFixtureRepoOnBranch(t *testing.T, branch string) (origin, work string) {
	t.Helper()
	fr := fixtureRunner()
	real := exec.NewRunner()
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	work = filepath.Join(root, "work")

	mustRunGit(t, fr, root, "init", "--bare", origin)
	mustRunGit(t, fr, root, "init", work)
	mustRunGit(t, fr, work, "config", "user.email", "test@example.com")
	mustRunGit(t, fr, work, "config", "user.name", "Test")
	mustRunGit(t, real, work, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustRunGit(t, real, work, "add", "-A")
	mustRunGit(t, real, work, "commit", "-m", "initial commit")
	mustRunGit(t, fr, work, "remote", "add", "origin", origin)
	mustRunGit(t, real, work, "push", "-u", "origin", "main")

	mustRunGit(t, real, work, "checkout", "-b", branch)
	return origin, work
}

// TestIntegration_ShipTail drives a hand-constructed fixture ticket
// (every stage before output already resolved by hand — the plan's own
// "driven by hand" wording, since render-drift through review have no
// CLI-only driver and no skill until phases 15-16) through
// `summary render -> stage start ship -> stage approve ship ->
// git commit -> pr create` against a real temporary git repository and
// a fixture GitHub transport. Phase 8's own "Done when" line, minus the
// stages phases 9-16 build the drivers for.
func TestIntegration_ShipTail(t *testing.T) {
	home := t.TempDir()
	const key = "PROJ-1"
	const branch = "feature/PROJ-1"

	_, work := newFixtureRepoOnBranch(t, branch)
	if err := os.WriteFile(filepath.Join(work, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatalf("WriteFile app.go: %v", err)
	}

	writeValidDoctorConfig(t, home) // host.default_base_branch = "main", matching the fixture repo
	ticketDir := filepath.Join(home, key)
	writeTicketFixture(t, ticketDir, key, "Add app", "desc", branch)
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.app", LocalPath: work, InstructionsFile: "AGENTS.md"},
	})
	if err := os.WriteFile(filepath.Join(ticketDir, "commit-message.md"), []byte("Add app\n"), 0o600); err != nil {
		t.Fatalf("write commit-message.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "pr-body.md"), []byte("PR body text\n"), 0o600); err != nil {
		t.Fatalf("write pr-body.md: %v", err)
	}

	t.Setenv("AI_FLOW_GITHUB_TOKEN", "tok")
	var postCount int
	useScriptedGitHub(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/"):
			return jsonResponse(200, "[]"), nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls"):
			postCount++
			return jsonResponse(201, `{"number":1,"html_url":"https://example.invalid/pull/1","draft":false}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
			return nil, nil
		}
	})

	// Gate transitions refused out of order, both directions.
	if _, _, exitCode := runRoot(t, "stage", "approve", key, "ship", "--home", home); exitCode != 3 {
		t.Fatalf("stage approve ship before stage start ship: exitCode = %d, want 3 (stage_not_allowed)", exitCode)
	}
	if _, _, exitCode := runRoot(t, "git", "commit", key, "--home", home); exitCode != 3 {
		t.Fatalf("git commit before ship approval: exitCode = %d, want 3 (approval_required)", exitCode)
	}

	// summary render -> output stage pass, the precondition
	// stage approve ship checks.
	if stdout, _, exitCode := runRoot(t, "summary", "render", key, "--home", home); exitCode != 0 {
		t.Fatalf("summary render: exitCode = %d\n%s", exitCode, stdout.String())
	}

	if stdout, _, exitCode := runRoot(t, "stage", "start", key, "ship", "--home", home); exitCode != 0 {
		t.Fatalf("stage start ship: exitCode = %d\n%s", exitCode, stdout.String())
	}
	if stdout, _, exitCode := runRoot(t, "stage", "approve", key, "ship", "--home", home); exitCode != 0 {
		t.Fatalf("stage approve ship: exitCode = %d\n%s", exitCode, stdout.String())
	}

	if stdout, _, exitCode := runRoot(t, "git", "commit", key, "--home", home); exitCode != 0 {
		t.Fatalf("git commit: exitCode = %d\n%s", exitCode, stdout.String())
	}
	log := mustRunGit(t, fixtureRunner(), work, "log", "-1", "--pretty=%s")
	if strings.TrimSpace(log.Stdout) != "Add app" {
		t.Errorf("HEAD commit message = %q, want %q", strings.TrimSpace(log.Stdout), "Add app")
	}

	stdout, _, exitCode := runRoot(t, "pr", "create", key, "--home", home)
	if exitCode != 0 {
		t.Fatalf("pr create: exitCode = %d\n%s", exitCode, stdout.String())
	}
	if postCount != 1 {
		t.Errorf("POST /pulls called %d times, want 1", postCount)
	}

	// Byte-stability: an independent second home, given the same
	// repo.json/mutation-results.json inputs, renders byte-identical
	// output.md.
	home2 := t.TempDir()
	ticketDir2 := filepath.Join(home2, key)
	writeTicketFixture(t, ticketDir2, key, "Add app", "desc", branch)
	writeRepoFixture(t, ticketDir2, "Team Beta", []repoArtifactRepository{
		{Name: "acme.app", LocalPath: work, InstructionsFile: "AGENTS.md"},
	})
	if _, _, exitCode := runRoot(t, "summary", "render", key, "--home", home2); exitCode != 0 {
		t.Fatalf("second summary render: exitCode = %d", exitCode)
	}

	b1, err := os.ReadFile(filepath.Join(ticketDir, "output.md"))
	if err != nil {
		t.Fatalf("read first output.md: %v", err)
	}
	b2, err := os.ReadFile(filepath.Join(ticketDir2, "output.md"))
	if err != nil {
		t.Fatalf("read second output.md: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("output.md is not byte-stable across two independent runs:\n--- first ---\n%s\n--- second ---\n%s", b1, b2)
	}
}
