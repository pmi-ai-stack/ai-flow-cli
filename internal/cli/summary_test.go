package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

func TestSummaryRender_MissingRepoJSON(t *testing.T) {
	home := t.TempDir()
	writeTicketFixture(t, filepath.Join(home, "PROJ-1"), "PROJ-1", "s", "d", "feature/PROJ-1")

	stdout, _, exitCode := runRoot(t, "summary", "render", "PROJ-1", "--home", home)
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

func TestSummaryRender_WritesOutputAndCompletesStage(t *testing.T) {
	home := t.TempDir()
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: "/work/acme.billing.api", InstructionsFile: "AGENTS.md"},
	})

	stdout, _, exitCode := runRoot(t, "summary", "render", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "output.md"))
	if err != nil {
		t.Fatalf("read output.md: %v", err)
	}
	if !strings.Contains(string(data), "# Summary") || !strings.Contains(string(data), "acme.billing.api") {
		t.Errorf("output.md missing expected content:\n%s", data)
	}

	st, err := handoff.LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if got := st.Entry(handoff.StageOutput).State; got != handoff.StatePass {
		t.Errorf("output stage state = %s, want pass", got)
	}
}

func TestSummaryRender_IdempotentOnRerun(t *testing.T) {
	home := t.TempDir()
	ticketDir := filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", nil)

	if _, _, exitCode := runRoot(t, "summary", "render", "PROJ-1", "--home", home); exitCode != 0 {
		t.Fatalf("first run: exitCode = %d", exitCode)
	}
	stdout, _, exitCode := runRoot(t, "summary", "render", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("second run: exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	st, err := handoff.LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if got := st.Entry(handoff.StageOutput).State; got != handoff.StatePass {
		t.Errorf("output stage state = %s, want pass", got)
	}
}
