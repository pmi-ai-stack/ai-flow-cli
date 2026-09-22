package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// fakeVisualUnlistedRunner is a *exec.UnlistedRunner test double for
// `visual run`'s CLI layer: StartUnlisted returns a real (but inert)
// short-lived process — exec.Process's fields are unexported, so a
// fake cannot fabricate one directly, the same constraint
// internal/verify/visual's own devserver_test.go works around the same
// way — and RunUnlisted simulates a capture script by writing the
// requested output file.
type fakeVisualUnlistedRunner struct{}

func (f *fakeVisualUnlistedRunner) StartUnlisted(context.Context, string, string, []string, []string) (*exec.Process, error) {
	r := exec.NewRunner()
	return r.StartUnlisted(context.Background(), ".", "sh", []string{"-c", "sleep 5"}, nil)
}

func (f *fakeVisualUnlistedRunner) RunUnlisted(_ context.Context, _ string, _ string, _, env []string, _ time.Duration) (exec.Output, error) {
	for _, e := range env {
		if out, ok := strings.CutPrefix(e, "AI_FLOW_CAPTURE_OUT="); ok {
			if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
				return exec.Output{}, err
			}
			if err := os.WriteFile(out, []byte("fake-png"), 0o600); err != nil {
				return exec.Output{}, err
			}
		}
	}
	return exec.Output{}, nil
}

func useFakeUnlistedRunner(t *testing.T) {
	t.Helper()
	orig := newUnlistedRunner
	newUnlistedRunner = func() exec.UnlistedRunner { return &fakeVisualUnlistedRunner{} }
	t.Cleanup(func() { newUnlistedRunner = orig })
}

func writeVisualConfig(t *testing.T, repoDir, devServerURL string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repoDir, ".ai-flow"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := `{
  "dev_server": { "command": ["npm", "run", "dev"], "url": "` + devServerURL + `", "startup_timeout_seconds": 5 },
  "capture_command": ["node", "capture.js"]
}`
	if err := os.WriteFile(filepath.Join(repoDir, ".ai-flow", "visual.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestVisualRunCapturesNamedScreens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	repoDir := t.TempDir()
	writeVisualConfig(t, repoDir, srv.URL)
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", repoDir}}, nil)
	useFakeUnlistedRunner(t)

	stdout, _, exitCode := runRoot(t, "visual", "run", "PROJ-1", "--screen", "/dashboard", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result visualRunResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	path, ok := env.Result.Screens["/dashboard"]
	if !ok || path == "" {
		t.Fatalf("Screens = %+v, want /dashboard present", env.Result.Screens)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("screenshot not written at %s: %v", path, err)
	}
}

func TestVisualRunNoVisualConfig(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", t.TempDir()}}, nil)

	stdout, _, exitCode := runRoot(t, "visual", "run", "PROJ-1", "--home", home)
	if exitCode != 4 {
		t.Fatalf("exitCode = %d, want 4 (tool_missing)\n%s", exitCode, stdout.String())
	}
}

func TestVisualJudgeRecordsVerdictAndScreenshotPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	repoDir := t.TempDir()
	writeVisualConfig(t, repoDir, srv.URL)
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", repoDir}}, nil)
	useFakeUnlistedRunner(t)

	if _, _, ec := runRoot(t, "visual", "run", "PROJ-1", "--screen", "/dashboard", "--home", home); ec != 0 {
		t.Fatalf("visual run: exitCode = %d", ec)
	}

	stdout, _, exitCode := runRoot(t, "visual", "judge", "PROJ-1", "--screen", "/dashboard", "--verdict", "pass", "--reason", "matches the mock", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result visualTestResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Screens) != 1 {
		t.Fatalf("Screens = %+v, want 1 entry", env.Result.Screens)
	}
	s := env.Result.Screens[0]
	if s.Screen != "/dashboard" || s.Verdict != "pass" || s.Reason != "matches the mock" || s.Screenshot == "" {
		t.Errorf("Screens[0] = %+v", s)
	}
}

func TestVisualJudgeUpsertsWithoutDisturbingOtherScreens(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	if _, _, ec := runRoot(t, "visual", "judge", key, "--screen", "/home", "--verdict", "pass", "--reason", "fine", "--home", home); ec != 0 {
		t.Fatalf("first judge: exitCode = %d", ec)
	}
	if _, _, ec := runRoot(t, "visual", "judge", key, "--screen", "/dashboard", "--verdict", "fail", "--reason", "broken layout", "--home", home); ec != 0 {
		t.Fatalf("second judge: exitCode = %d", ec)
	}
	// Re-judging /home with a different verdict must replace, not
	// duplicate, its entry.
	stdout, _, exitCode := runRoot(t, "visual", "judge", key, "--screen", "/home", "--verdict", "fail", "--reason", "regressed", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	var env struct {
		Result visualTestResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Result.Screens) != 2 {
		t.Fatalf("Screens = %+v, want 2 entries (no duplicate /home)", env.Result.Screens)
	}
	byScreen := map[string]visualTestResultsScreen{}
	for _, s := range env.Result.Screens {
		byScreen[s.Screen] = s
	}
	if byScreen["/home"].Verdict != "fail" || byScreen["/home"].Reason != "regressed" {
		t.Errorf("/home = %+v, want the replaced verdict/reason", byScreen["/home"])
	}
	if byScreen["/dashboard"].Verdict != "fail" {
		t.Errorf("/dashboard = %+v, want untouched", byScreen["/dashboard"])
	}
}

func TestVisualJudgeRequiresFlags(t *testing.T) {
	home := t.TempDir()
	cases := [][]string{
		{"visual", "judge", "PROJ-1", "--verdict", "pass", "--reason", "x", "--home", home},
		{"visual", "judge", "PROJ-1", "--screen", "/home", "--reason", "x", "--home", home},
		{"visual", "judge", "PROJ-1", "--screen", "/home", "--verdict", "maybe", "--reason", "x", "--home", home},
		{"visual", "judge", "PROJ-1", "--screen", "/home", "--verdict", "pass", "--home", home},
	}
	for _, args := range cases {
		_, _, exitCode := runRoot(t, args...)
		if exitCode != 2 {
			t.Errorf("args=%v: exitCode = %d, want 2 (usage)", args, exitCode)
		}
	}
}
