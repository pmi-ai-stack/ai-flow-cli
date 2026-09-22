package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// approveShip writes a status.json with the ship stage already
// approved — the precondition git commit/push/pr create require.
func approveShip(t *testing.T, home, key string) {
	t.Helper()
	st := handoff.NewStatus(key)
	e := st.Entry(handoff.StageShip)
	e.State = handoff.StateRunning
	e.Approval = &handoff.Approval{By: "test", At: "2026-01-01T00:00:00Z", Gate: handoff.GatePre}
	if _, err := handoff.SaveStatus(home, st); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}
}

func TestGitCommit_RefusesWithoutShipApproval(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", nil)

	stdout, _, exitCode := runRoot(t, "git", "commit", "PROJ-1", "--home", home)
	if exitCode != 3 {
		t.Fatalf("exitCode = %d, want 3\n%s", exitCode, stdout.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.Error.Code != "approval_required" {
		t.Errorf("error.code = %q, want approval_required", env.Error.Code)
	}
}

func TestGitPush_RefusesWithoutShipApproval(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false)
	writeTicketFixture(t, filepath.Join(home, "PROJ-1"), "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, filepath.Join(home, "PROJ-1"), "Team Beta", nil)

	_, _, exitCode := runRoot(t, "git", "push", "PROJ-1", "--home", home)
	if exitCode != 3 {
		t.Fatalf("exitCode = %d, want 3", exitCode)
	}
}

func TestGitCommit_ApprovedCommitsDirtyReposOnly(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	apiDir := repoInstructionsDir(t, localBase, "acme.billing.api")
	webDir := repoInstructionsDir(t, localBase, "acme.billing.web")
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: apiDir, InstructionsFile: "AGENTS.md"},
		{Name: "acme.billing.web", LocalPath: webDir, InstructionsFile: "AGENTS.md"},
	})
	if err := os.WriteFile(filepath.Join(ticketDir, "commit-message.md"), []byte("fix: a thing\n"), 0o600); err != nil {
		t.Fatalf("write commit-message.md: %v", err)
	}
	approveShip(t, home, "PROJ-1")

	se := newScriptedExecer(t).
		On("git", []string{"status", "--porcelain"}, exec.Output{Stdout: " M a.go\n"}).
		On("git", []string{"add", "-A"}, exec.Output{}).
		On("git", []string{"commit", "-m", "fix: a thing\n"}, exec.Output{})
	// Both repos share the same fake script since scriptedExecer
	// matches on tool+args only, not directory — acceptable here
	// because the assertion is on the result shape, not on which
	// directory each call targeted (gitops's own tests already prove
	// Dir is threaded through correctly).
	useScriptedExecer(t, se)

	stdout, _, exitCode := runRoot(t, "git", "commit", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	var env struct {
		Result gitCommitResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Repositories) != 2 {
		t.Fatalf("repositories = %+v, want 2 entries", env.Result.Repositories)
	}
	for _, r := range env.Result.Repositories {
		if !r.Committed {
			t.Errorf("repo %s: committed = false, want true", r.Repo)
		}
	}
}

func TestGitCommit_MissingCommitMessageIsMissingArtifact(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", nil)
	approveShip(t, home, "PROJ-1")

	stdout, _, exitCode := runRoot(t, "git", "commit", "PROJ-1", "--home", home)
	if exitCode != 3 {
		t.Fatalf("exitCode = %d, want 3\n%s", exitCode, stdout.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.Error.Code != "missing_artifact" {
		t.Errorf("error.code = %q, want missing_artifact", env.Error.Code)
	}
}

func TestGitPrepare_DirtyWorktreeProducesNeedsInput(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	apiDir := repoInstructionsDir(t, localBase, "acme.billing.api")
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: apiDir, InstructionsFile: "AGENTS.md"},
	})

	se := newScriptedExecer(t).On("git", []string{"status", "--porcelain"}, exec.Output{Stdout: " M dirty.go\n"})
	useScriptedExecer(t, se)

	stdout, _, exitCode := runRoot(t, "git", "prepare", "PROJ-1", "--home", home)
	if exitCode != 4 {
		t.Fatalf("exitCode = %d, want 4\n%s", exitCode, stdout.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		NeedsInput struct {
			ResumeWith string `json:"resume_with"`
		} `json:"needs_input"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.Error.Code != "dirty_worktree" {
		t.Errorf("error.code = %q, want dirty_worktree", env.Error.Code)
	}
	if env.NeedsInput.ResumeWith != "git prepare PROJ-1" {
		t.Errorf("needs_input.resume_with = %q, want %q", env.NeedsInput.ResumeWith, "git prepare PROJ-1")
	}
}

func TestGitPrepare_SimulateMakesNoExecCall(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	apiDir := repoInstructionsDir(t, localBase, "acme.billing.api")
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: apiDir, InstructionsFile: "AGENTS.md"},
	})

	// No responses registered at all — any Run call fails the test.
	useScriptedExecer(t, newScriptedExecer(t))

	stdout, _, exitCode := runRoot(t, "git", "prepare", "PROJ-1", "--home", home, "--simulate")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result gitPrepareSimulateResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.Result.Branch != "feature/PROJ-1" || env.Result.Base != "main" {
		t.Errorf("result = %+v", env.Result)
	}
}
