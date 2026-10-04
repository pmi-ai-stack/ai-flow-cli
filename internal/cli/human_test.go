package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff/handofftest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func humanOut(t *testing.T, wantExit int, args ...string) string {
	t.Helper()
	stdout, _, ec := runRoot(t, append(args, "--human")...)
	if ec != wantExit {
		t.Fatalf("%v --human: exitCode = %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	return stdout.String()
}

func noJSON(t *testing.T, out string) {
	t.Helper()
	if strings.ContainsAny(out, "{}") || strings.Contains(out, `":`) {
		t.Errorf("human output still contains JSON:\n%s", out)
	}
}

func TestHumanStatusShowsStagesAndNextStep(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "PROJ-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	out := humanOut(t, 0, "status", "PROJ-1", "--home", home)
	noJSON(t, out)
	if !strings.HasPrefix(out, "status: ok\n") {
		t.Errorf("output does not start with the command header:\n%s", out)
	}
	for _, want := range []string{"PROJ-1", filepath.Join(home, "PROJ-1")} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !regexp.MustCompile(`(?m)^\s+fetch\s+pending$`).MatchString(out) {
		t.Errorf("no aligned `fetch  pending` row:\n%s", out)
	}
	if !regexp.MustCompile(`(?m)^next: `).MatchString(out) {
		t.Errorf("no `next:` line:\n%s", out)
	}
}

func TestHumanStandardsCheckListsOpenFindings(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: %d", ec)
	}
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "AGENTS.md"), "# repo\n")
	writeFileTest(t, filepath.Join(repoDir, "go.mod"), "module example.com/acme\n\ngo 1.27\n")
	x := scriptEmbeddedGoChecks(t, `{"Issues":[{"Text":"unused var","Pos":{"Filename":"main.go","Line":3},"FromLinter":"unused"}]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	out := humanOut(t, 6, "standards", "check", repoDir, "--home", home)
	// Exit 6 is a failure envelope: the generic failure rendering must be unchanged.
	if !strings.Contains(out, "failed (invalid_result)") {
		t.Errorf("failure rendering changed:\n%s", out)
	}
}

func TestHumanStandardsCheckCleanAndWithFindings(t *testing.T) {
	// A clean directory run: success path, the renderer's own output.
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: %d", ec)
	}
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "AGENTS.md"), "# repo\n")
	writeFileTest(t, filepath.Join(repoDir, "go.mod"), "module example.com/acme\n\ngo 1.27\n")
	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	out := humanOut(t, 0, "standards", "check", repoDir, "--home", home)
	noJSON(t, out)
	if !strings.HasPrefix(out, "standards check: ok\n") || !strings.Contains(out, "0 open") {
		t.Errorf("clean run output:\n%s", out)
	}
	if !strings.Contains(out, repoDir) {
		t.Errorf("output does not name the checked directory:\n%s", out)
	}
}

func TestHumanStandardsCheckTicketShowsFindingDetails(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-9")
	x := scriptEmbeddedGoChecks(t, `{"Issues":[{"Text":"unused var","Pos":{"Filename":"main.go","Line":3},"FromLinter":"unused"}]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	out := humanOut(t, 0, "standards", "check", "PROJ-9", "--home", home)
	noJSON(t, out)
	re := regexp.MustCompile(`(?m)^(MUST|SHOULD|MAY)\s+\S+\s+main\.go:3$`)
	if !re.MatchString(out) {
		t.Errorf("no `SEVERITY check-id main.go:3` line:\n%s", out)
	}
	if !strings.Contains(out, "unused var") || !strings.Contains(out, "1 open") {
		t.Errorf("message or count missing:\n%s", out)
	}
}

func TestHumanDoctorSummarisesAndKeepsWarnings(t *testing.T) {
	home := t.TempDir()
	writeRoutingTestConfig(t, home, t.TempDir(), false)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "")
	t.Setenv("AI_FLOW_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	// An expired waiver in the active ticket's repository: the warning
	// must still be printed, as `! ...`, after the summary.
	handofftest.FixedClock(t, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	repo := filepath.Join(t.TempDir(), "payments")
	writeFileTest(t, filepath.Join(repo, ".ai-flow", "waivers.json"), waiversFixture)
	activeWithRepo(t, home, repo)
	useScriptedJira(t, func(*http.Request) (*http.Response, error) { return jsonResponse(200, `{}`), nil })

	out := humanOut(t, 0, "doctor", "--home", home)
	noJSON(t, out)
	if !strings.HasPrefix(out, "doctor: ok\n") {
		t.Errorf("header:\n%s", out)
	}
	for _, re := range []string{
		`(?m)^config\s+INVALID: `,
		`(?m)^\s+- ops\.default_environment`,
		`(?m)^tracker\s+credentials not set`,
		`(?m)^active ticket\s+PROJ-1`,
		`(?m)^! waivers: payments: waiver for check go\.lint expired`,
	} {
		if !regexp.MustCompile(re).MatchString(out) {
			t.Errorf("no line matching %q in:\n%s", re, out)
		}
	}
}

// Every other command keeps today's rendering exactly: the header and
// the result as indented JSON.
func TestHumanFallsBackForUnregisteredCommands(t *testing.T) {
	out := humanOut(t, 0, "version")
	if !strings.HasPrefix(out, "version: ok\n{\n") {
		t.Errorf("generic rendering changed for `version`:\n%s", out)
	}
}

func TestJSONStillWinsOverHuman(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "PROJ-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	stdout, _, ec := runRoot(t, "status", "PROJ-1", "--home", home, "--human", "--json")
	if ec != 0 || !strings.HasPrefix(stdout.String(), "{\n") {
		t.Errorf("--json did not win over --human (exit %d):\n%s", ec, stdout.String())
	}
}

func TestHumanDoctorValidConfig(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: %d", ec)
	}
	out := humanOut(t, 0, "doctor", "--home", home)
	noJSON(t, out)
	if !regexp.MustCompile(`(?m)^config\s+valid  \S*config\.json$`).MatchString(out) {
		t.Errorf("no `config  valid  <path>` row:\n%s", out)
	}
	if !regexp.MustCompile(`(?m)^active ticket\s+none$`).MatchString(out) {
		t.Errorf("no `active ticket  none` row:\n%s", out)
	}
}
