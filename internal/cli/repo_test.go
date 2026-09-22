package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func TestRepoLocate_ResolvesAndCompletesLocateStage(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	repoInstructionsDir(t, localBase, "acme.billing.api")
	repoInstructionsDir(t, localBase, "acme.billing.web")
	writeRoutingTestConfig(t, home, localBase, false)
	writeTicketFixture(t, filepath.Join(home, "PROJ-1"), "PROJ-1", "fix acme.billing.api retry logic", "a billing bug", "bugfix/PROJ-1")

	stdout, _, exitCode := runRoot(t, "repo", "locate", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(home, "PROJ-1", "repo.json"))
	if err != nil {
		t.Fatalf("repo.json not written: %v", err)
	}
	var artifact repoArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("repo.json invalid: %v", err)
	}
	if artifact.Team != "Team Beta" {
		t.Errorf("team = %q, want Team Beta", artifact.Team)
	}
	if len(artifact.Repositories) != 1 || artifact.Repositories[0].Name != "acme.billing.api" {
		t.Fatalf("repositories = %+v, want exactly acme.billing.api", artifact.Repositories)
	}
	if artifact.Repositories[0].InstructionsFile != "AGENTS.md" {
		t.Errorf("instructions_file = %q, want AGENTS.md", artifact.Repositories[0].InstructionsFile)
	}

	st, err := handoff.LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.Entry(handoff.StageLocate).State != handoff.StatePass {
		t.Errorf("locate stage state = %s, want pass", st.Entry(handoff.StageLocate).State)
	}
}

func TestRepoLocate_AmbiguousProducesNeedsInput(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	repoInstructionsDir(t, localBase, "acme.billing.api")
	repoInstructionsDir(t, localBase, "acme.billing.web")
	writeRoutingTestConfig(t, home, localBase, false)
	writeTicketFixture(t, filepath.Join(home, "PROJ-1"), "PROJ-1", "a change with no matching keyword", "no repo named here either", "feature/PROJ-1")

	stdout, _, exitCode := runRoot(t, "repo", "locate", "PROJ-1", "--home", home)
	if exitCode != 4 {
		t.Fatalf("exitCode = %d, want 4\n%s", exitCode, stdout.String())
	}

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		NeedsInput struct {
			Question   string   `json:"question"`
			Options    []string `json:"options"`
			ResumeWith string   `json:"resume_with"`
		} `json:"needs_input"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error.Code != "routing_ambiguous" {
		t.Errorf("error.code = %q, want routing_ambiguous", env.Error.Code)
	}
	if len(env.NeedsInput.Options) != 2 {
		t.Errorf("needs_input.options = %v, want both billing repos", env.NeedsInput.Options)
	}
	if env.NeedsInput.ResumeWith != "repo confirm PROJ-1 --repos <REPO>" {
		t.Errorf("needs_input.resume_with = %q", env.NeedsInput.ResumeWith)
	}

	if _, err := os.Stat(filepath.Join(home, "PROJ-1", "repo.json")); err == nil {
		t.Error("repo.json should not be written when routing is ambiguous")
	}
}

func TestRepoLocate_SimulateWritesNothing(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	repoInstructionsDir(t, localBase, "acme.billing.api")
	repoInstructionsDir(t, localBase, "acme.billing.web")
	writeRoutingTestConfig(t, home, localBase, false)
	writeTicketFixture(t, filepath.Join(home, "PROJ-1"), "PROJ-1", "fix acme.billing.api retry logic", "desc", "bugfix/PROJ-1")

	_, _, exitCode := runRoot(t, "repo", "locate", "PROJ-1", "--home", home, "--simulate")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if _, err := os.Stat(filepath.Join(home, "PROJ-1", "repo.json")); err == nil {
		t.Error("--simulate should not write repo.json")
	}
	st, err := handoff.LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.Entry(handoff.StageLocate).State != handoff.StatePending {
		t.Errorf("locate stage state = %s, want pending (--simulate must not drive the stage)", st.Entry(handoff.StageLocate).State)
	}
}

func TestRepoConfirm_DerivesTeamAndWritesArtifact(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	repoInstructionsDir(t, localBase, "acme.billing.api")
	repoInstructionsDir(t, localBase, "acme.billing.web")
	writeRoutingTestConfig(t, home, localBase, false)
	writeTicketFixture(t, filepath.Join(home, "PROJ-1"), "PROJ-1", "s", "d", "feature/PROJ-1")

	stdout, _, exitCode := runRoot(t, "repo", "confirm", "PROJ-1", "--home", home,
		"--repos", "acme.billing.api", "--repos", "acme.billing.web")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	var artifact repoArtifact
	data, err := os.ReadFile(filepath.Join(home, "PROJ-1", "repo.json"))
	if err != nil {
		t.Fatalf("repo.json not written: %v", err)
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("repo.json invalid: %v", err)
	}
	if len(artifact.Repositories) != 2 {
		t.Fatalf("repositories = %+v, want both", artifact.Repositories)
	}

	st, err := handoff.LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if st.Entry(handoff.StageLocate).State != handoff.StatePass {
		t.Errorf("locate stage state = %s, want pass", st.Entry(handoff.StageLocate).State)
	}
}

func TestRepoScope_RewritesInScopeFiles(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	apiDir := repoInstructionsDir(t, localBase, "acme.billing.api")
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: apiDir, InstructionsFile: "AGENTS.md"},
	})

	se := newScriptedExecer(t).
		On("git", []string{"diff", "origin/main...HEAD", "--name-only"}, exec.Output{Stdout: "a.go\nb.go\n"}).
		On("git", []string{"ls-files", "--others", "--exclude-standard"}, exec.Output{Stdout: "c.go\n"}).
		On("git", []string{"diff", "origin/main...HEAD"}, exec.Output{Stdout: "diff text"})
	useScriptedExecer(t, se)

	stdout, _, exitCode := runRoot(t, "repo", "scope", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "repo.json"))
	if err != nil {
		t.Fatalf("repo.json not found: %v", err)
	}
	var artifact repoArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("repo.json invalid: %v", err)
	}
	want := []string{"a.go", "b.go", "c.go"}
	if len(artifact.InScopeFiles) != len(want) {
		t.Fatalf("in_scope_files = %v, want %v", artifact.InScopeFiles, want)
	}
	for i, w := range want {
		if artifact.InScopeFiles[i] != w {
			t.Errorf("in_scope_files[%d] = %q, want %q", i, artifact.InScopeFiles[i], w)
		}
	}
}
