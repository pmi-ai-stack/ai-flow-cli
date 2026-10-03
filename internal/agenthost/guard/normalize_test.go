package guard_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/guard"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// realLayout is a real on-disk home and repository, so symlinks and
// relative paths resolve against something — the bypasses below only
// exist on a real filesystem.
type realLayout struct {
	home, repo, ticketDir string
	active                *handoff.Active
}

func newRealLayout(t *testing.T) realLayout {
	t.Helper()
	root := t.TempDir()
	l := realLayout{home: filepath.Join(root, "home", ".ai-flow"), repo: filepath.Join(root, "work", "acme.api")}
	l.ticketDir = handoff.TicketDir(l.home, testKey)
	for _, d := range []string{l.ticketDir, l.repo} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	l.active = freshActive(func(a *handoff.Active) { a.RepoRoots = []string{l.repo} })
	return l
}

func (l realLayout) write(path, cwd string) agenthost.Decision {
	return l.writeHome(path, cwd, l.home)
}

func (l realLayout) writeHome(path, cwd, home string) agenthost.Decision {
	return guard.Evaluate(guard.Input{
		Event:  agenthost.HookEvent{Host: agenthost.HostClaudeCode, Event: agenthost.EventWrite, FilePath: path, Cwd: cwd},
		Active: l.active, Home: home, Idle: testIdle, Now: testNow,
	})
}

// TestGuardRelativeFilePathResolvesAgainstCwd: "status.json" written
// from inside the ticket directory is the ticket directory's
// status.json, not an unrelated relative path that matches nothing.
func TestGuardRelativeFilePathResolvesAgainstCwd(t *testing.T) {
	l := newRealLayout(t)
	requireDeny(t, l.write("status.json", l.ticketDir))
	requireDeny(t, l.write(filepath.Join("..", testKey, "status.json"), filepath.Join(l.home, "cache")))
	requireAllow(t, l.write("plan.md", l.ticketDir)) // a prose artifact stays writable
}

// TestGuardSymlinkIntoTicketDirIsDenied: a path that only reaches the
// ticket directory through a symlink is still the ticket directory —
// including a file that does not exist yet (the common case: a new
// write), which EvalSymlinks alone would fail on.
func TestGuardSymlinkIntoTicketDirIsDenied(t *testing.T) {
	l := newRealLayout(t)
	link := filepath.Join(l.repo, "handoff")
	if err := os.Symlink(l.ticketDir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	requireDeny(t, l.write(filepath.Join(link, "status.json"), l.repo))
	requireDeny(t, l.write(filepath.Join(link, "new-dir", "not-yet.json"), l.repo))
}

// TestGuardRelativeHomeIsNormalized: `--home .ai-flow` from the parent
// directory names the same home as its absolute path.
func TestGuardRelativeHomeIsNormalized(t *testing.T) {
	l := newRealLayout(t)
	t.Chdir(filepath.Dir(l.home))
	requireDeny(t, l.writeHome(filepath.Join(l.ticketDir, "status.json"), l.repo, ".ai-flow"))
	requireDeny(t, l.writeHome(filepath.Join(l.home, "cache", "x"), l.repo, ".ai-flow"))
}

// TestGuardSymlinkedRepoRootStillScopes: a repository root recorded
// through a symlink (macOS /tmp → /private/tmp is the everyday case)
// and a cwd spelled the other way still compare equal.
func TestGuardSymlinkedRepoRootStillScopes(t *testing.T) {
	l := newRealLayout(t)
	alias := filepath.Join(filepath.Dir(l.repo), "alias")
	if err := os.Symlink(l.repo, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	l.active.RepoRoots = []string{alias}
	d := guard.Evaluate(guard.Input{
		Event:  agenthost.HookEvent{Host: agenthost.HostClaudeCode, Event: agenthost.EventShell, Command: "git push", Cwd: l.repo},
		Active: l.active, Home: l.home, Idle: testIdle, Now: testNow,
	})
	requireDeny(t, d)
}

// TestGuardShellToolSpelling is SEC-8: an absolute tool path and a
// leading VAR=value assignment are the same tool. Not a sandbox — `env
// git push` and `sh -c` remain out of scope, by the guard's own
// definition.
func TestGuardShellToolSpelling(t *testing.T) {
	for _, cmd := range []string{"/usr/bin/git push", "GIT_TRACE=1 git push origin main", "A=1 B=2 /usr/local/bin/git commit -m x", "/usr/bin/terraform apply"} {
		t.Run(cmd, func(t *testing.T) {
			requireDeny(t, evalShell(cmd, testRepo, freshActive(nil)))
		})
	}
	requireAllow(t, evalShell("GIT_TRACE=1 git status", testRepo, freshActive(nil)))
}
