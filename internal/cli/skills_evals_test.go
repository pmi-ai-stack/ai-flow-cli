package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// useSequencedEvalRunner overrides the shared newUnlistedRunner
// package var (visual_test.go's own established save/restore pattern)
// with an exectest.UnlistedScripted queued with outputs in order —
// the same scripted-by-call-order double internal/skills/evals's own
// tests use, shared rather than duplicated here.
func useSequencedEvalRunner(t *testing.T, outputs ...exec.Output) {
	t.Helper()
	orig := newUnlistedRunner
	fake := exectest.NewUnlisted(t)
	for _, out := range outputs {
		fake.Then(out)
	}
	newUnlistedRunner = func() exec.UnlistedRunner { return fake }
	t.Cleanup(func() { newUnlistedRunner = orig })
}

// buildEvalsFixtureSkill lays out dir/skills/<name>/ with a minimal
// SKILL.md, evals/evals.json (three scenarios, all resolving to
// key's ticket key so a single fixture directory serves every
// scenario) and one evals/fixtures/<scenario>/status.json per
// scenario.
func buildEvalsFixtureSkill(t *testing.T, root, name, key string) {
	t.Helper()
	skillDir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	evalsJSON := `{"scenarios": [
		{"scenario": "happy_path", "query": "load ` + name + ` for ` + key + `", "fixture": "fixtures/happy_path", "baseline": "b", "expected_behavior": ["e"]},
		{"scenario": "stop_and_ask", "query": "load ` + name + ` for ` + key + `", "fixture": "fixtures/stop_and_ask", "baseline": "b", "expected_behavior": ["e"]},
		{"scenario": "temptation", "query": "load ` + name + ` for ` + key + `", "fixture": "fixtures/temptation", "baseline": "b", "expected_behavior": ["e"]}
	]}`
	evalsDir := filepath.Join(skillDir, "evals")
	if err := os.MkdirAll(evalsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalsDir, "evals.json"), []byte(evalsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"happy_path", "stop_and_ask", "temptation"} {
		fixtureDir := filepath.Join(evalsDir, "fixtures", scenario)
		if err := os.MkdirAll(fixtureDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixtureDir, "status.json"), []byte(`{"stages":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func passingRunAndGrade() []exec.Output {
	pass := exec.Output{Stdout: `{"result":"{\"pass\": true, \"reason\": \"ok\"}"}`, ExitCode: 0}
	// Each scenario makes two RunUnlisted calls: the host run, then the
	// grade. Three scenarios per skill.
	return []exec.Output{
		{Stdout: `{"result":"ran"}`, ExitCode: 0}, pass,
		{Stdout: `{"result":"ran"}`, ExitCode: 0}, pass,
		{Stdout: `{"result":"ran"}`, ExitCode: 0}, pass,
	}
}

func TestSkillsEvals_SkipsWithoutCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	home := t.TempDir()
	dir := t.TempDir()
	buildEvalsFixtureSkill(t, dir, "hello", "PROJ-1")

	stdout, _, exitCode := runRoot(t, "skills", "evals", dir, "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0 (skipped, never failed)\n%s", exitCode, stdout.String())
	}

	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Skipped    bool   `json:"skipped"`
			SkipReason string `json:"skip_reason"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK || !env.Result.Skipped || env.Result.SkipReason == "" {
		t.Errorf("env = %+v, want ok:true, result.skipped:true with a reason", env)
	}
}

func TestSkillsEvals_PassingRun(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	home := t.TempDir()
	dir := t.TempDir()
	buildEvalsFixtureSkill(t, dir, "hello", "PROJ-1")
	useSequencedEvalRunner(t, passingRunAndGrade()...)

	outDir := t.TempDir()
	stdout, _, exitCode := runRoot(t, "skills", "evals", dir, "--home", home, "--out", outDir)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	if _, err := os.Stat(filepath.Join(outDir, "results.json")); err != nil {
		t.Errorf("results.json was not written to --out: %v", err)
	}

	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			PassRate float64 `json:"pass_rate"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Result.PassRate != 1.0 {
		t.Errorf("env = %+v, want ok:true, pass_rate:1.0", env)
	}
}

func TestSkillsEvals_SkillFilter(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	home := t.TempDir()
	dir := t.TempDir()
	buildEvalsFixtureSkill(t, dir, "hello", "PROJ-1")
	buildEvalsFixtureSkill(t, dir, "other", "PROJ-1")
	useSequencedEvalRunner(t, passingRunAndGrade()...) // only "hello"'s three scenarios should run

	_, _, exitCode := runRoot(t, "skills", "evals", dir, "--home", home, "--skill", "hello", "--out", t.TempDir())
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
}

func TestSkillsEvals_UnknownSkillFilterIsUsageError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	home := t.TempDir()
	dir := t.TempDir()
	buildEvalsFixtureSkill(t, dir, "hello", "PROJ-1")

	_, _, exitCode := runRoot(t, "skills", "evals", dir, "--home", home, "--skill", "no-such-skill")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)", exitCode)
	}
}

func TestSkillsEvals_FailingScenarioIsInvalidResult(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	home := t.TempDir()
	dir := t.TempDir()
	buildEvalsFixtureSkill(t, dir, "hello", "PROJ-1")

	failVerdict := exec.Output{Stdout: `{"result":"{\"pass\": false, \"reason\": \"missing behavior\"}"}`, ExitCode: 0}
	ran := exec.Output{Stdout: `{"result":"ran"}`, ExitCode: 0}
	useSequencedEvalRunner(t, ran, failVerdict, ran, failVerdict, ran, failVerdict)

	stdout, _, exitCode := runRoot(t, "skills", "evals", dir, "--home", home, "--out", t.TempDir())
	if exitCode != 6 {
		t.Fatalf("exitCode = %d, want 6 (invalid_result)\n%s", exitCode, stdout.String())
	}

	var env struct {
		Error *struct {
			Code    string   `json:"code"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.Error == nil || env.Error.Code != "invalid_result" || len(env.Error.Details) == 0 {
		t.Errorf("env.Error = %+v, want invalid_result with details", env.Error)
	}
}
