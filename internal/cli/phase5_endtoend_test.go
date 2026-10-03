package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// TestPhase5EndToEnd_LocateToPRCreate drives the phase-5 plan's own
// "Done when" sentence: a fixture ticket runs
// repo locate → git prepare → git commit → pr create with a recorded
// ship approval, and is refused at each gated step without one.
func TestPhase5EndToEnd_LocateToPRCreate(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	repoInstructionsDir(t, localBase, "acme.billing.api")
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "fix acme.billing.api retry logic", "desc", "bugfix/PROJ-1")

	// 1. repo locate resolves to exactly acme.billing.api and passes
	// the locate stage.
	if stdout, _, ec := runRoot(t, "repo", "locate", "PROJ-1", "--home", home); ec != 0 {
		t.Fatalf("repo locate: exitCode = %d\n%s", ec, stdout.String())
	}
	st, err := handoff.LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.Entry(handoff.StageLocate).State != handoff.StatePass {
		t.Fatalf("locate stage = %s, want pass", st.Entry(handoff.StageLocate).State)
	}

	// 2. git commit and pr create both refuse before a ship approval
	// exists.
	if err := os.WriteFile(filepath.Join(ticketDir, "commit-message.md"), []byte("fix: retry logic\n"), 0o600); err != nil {
		t.Fatalf("write commit-message.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketDir, "pr-body.md"), []byte("PR body\n"), 0o600); err != nil {
		t.Fatalf("write pr-body.md: %v", err)
	}
	if _, _, ec := runRoot(t, "git", "commit", "PROJ-1", "--home", home); ec != 3 {
		t.Fatalf("git commit before approval: exitCode = %d, want 3", ec)
	}
	if _, _, ec := runRoot(t, "pr", "create", "PROJ-1", "--home", home); ec != 3 {
		t.Fatalf("pr create before approval: exitCode = %d, want 3", ec)
	}

	// 3. git prepare (no approval needed for this stage).
	se := newScriptedExecer(t).
		On("git", []string{"status", "--porcelain"}, exec.Output{}).
		On("git", []string{"fetch", "--end-of-options", "origin", "main"}, exec.Output{}).
		On("git", []string{"rev-parse", "--verify", "--end-of-options", "main"}, exec.Output{}).
		On("git", []string{"rev-list", "--count", "--end-of-options", "origin/main..main"}, exec.Output{Stdout: "0\n"}).
		On("git", []string{"rev-parse", "--verify", "--end-of-options", "bugfix/PROJ-1"}, exec.Output{}).
		On("git", []string{"merge-base", "--end-of-options", "origin/main", "bugfix/PROJ-1"}, exec.Output{Stdout: "deadbeef\n"}).
		On("git", []string{"worktree", "add", "--detach", "--end-of-options", filepath.Join(home, "cache", "basewt", "PROJ-1", "acme.billing.api"), "deadbeef"}, exec.Output{}).
		On("git", []string{"add", "-A"}, exec.Output{}).
		On("git", []string{"commit", "-m", "fix: retry logic\n"}, exec.Output{}).
		On("git", []string{"push", "-u", "--end-of-options", "origin", "bugfix/PROJ-1"}, exec.Output{}).
		On("git", []string{"remote", "get-url", "origin"}, exec.Output{Stdout: "git@github.com:acme/billing-api.git\n"})
	useScriptedExecer(t, se)

	if stdout, _, ec := runRoot(t, "git", "prepare", "PROJ-1", "--home", home); ec != 0 {
		t.Fatalf("git prepare: exitCode = %d\n%s", ec, stdout.String())
	}
	// This test scripts every git call (design: prove the CLI's own
	// wiring — flags, gates, result shapes — not git mechanics, which
	// internal/gitops's own -tags integration suite already proves
	// against real git). The worktree's actual SHA and on-disk
	// existence are asserted there, not here.

	// 4. approve ship, then commit and pr create both succeed.
	approveShip(t, home, "PROJ-1")

	if stdout, _, ec := runRoot(t, "git", "commit", "PROJ-1", "--home", home); ec != 0 {
		t.Fatalf("git commit after approval: exitCode = %d\n%s", ec, stdout.String())
	}

	t.Setenv("AI_FLOW_GITHUB_TOKEN", "tok")
	var postCount int
	useScriptedGitHub(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/acme/billing-api/pulls"):
			return jsonResponse(200, "[]"), nil
		case r.Method == "POST" && r.URL.Path == "/repos/acme/billing-api/pulls":
			postCount++
			return jsonResponse(201, `{"number":1,"html_url":"https://github.com/acme/billing-api/pull/1","draft":false}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
			return nil, nil
		}
	})
	if stdout, _, ec := runRoot(t, "pr", "create", "PROJ-1", "--home", home); ec != 0 {
		t.Fatalf("pr create: exitCode = %d\n%s", ec, stdout.String())
	}
	if postCount != 1 {
		t.Fatalf("POST called %d times, want 1", postCount)
	}

	// 5. a second pr create reuses the PR — no second POST.
	useScriptedGitHub(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/acme/billing-api/pulls") {
			return jsonResponse(200, `[{"number":1,"html_url":"https://github.com/acme/billing-api/pull/1","draft":false}]`), nil
		}
		t.Fatalf("unexpected request on re-run %s %s (should have reused)", r.Method, r.URL.String())
		return nil, nil
	})
	stdout, _, ec := runRoot(t, "pr", "create", "PROJ-1", "--home", home)
	if ec != 0 {
		t.Fatalf("second pr create: exitCode = %d\n%s", ec, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "ship-results.json"))
	if err != nil {
		t.Fatalf("ship-results.json: %v", err)
	}
	var results shipResultsArtifact
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("ship-results.json invalid: %v", err)
	}
	if len(results.PullRequests) != 1 || !results.PullRequests[0].Reused {
		t.Errorf("pull_requests = %+v, want the single PR marked reused on the second create", results.PullRequests)
	}
}
