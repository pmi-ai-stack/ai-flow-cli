package evals

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// buildFixtureSkill lays out pluginRoot/skills/<name>/ with a minimal
// SKILL.md (for StageSkills) and evals/fixtures/<scenario>/status.json
// (for StageFixture) — everything RunScenario needs to stage a
// workspace, without a real skill body or a real handoff artifact set.
func buildFixtureSkill(t *testing.T, pluginRoot, name, scenario string) string {
	t.Helper()
	dir := filepath.Join(pluginRoot, "skills", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(dir, "evals", "fixtures", scenario)
	if err := os.MkdirAll(fixtureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "status.json"), []byte(`{"stages":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func passingGraderOutput() exec.Output {
	return exec.Output{Stdout: `{"result":"{\"pass\": true, \"reason\": \"ok\"}"}`, ExitCode: 0}
}

func TestRunScenario_Pass(t *testing.T) {
	pluginRoot := t.TempDir()
	skillDir := buildFixtureSkill(t, pluginRoot, "hello", "happy_path")

	x := exectest.NewUnlisted(t).
		Then(exec.Output{Stdout: `{"result":"ran fine"}`, ExitCode: 0}). // host run
		Then(passingGraderOutput())                                      // grade

	s := Scenario{
		Kind:             KindHappyPath,
		Query:            "load the hello skill PROJ-1",
		Fixture:          "fixtures/happy_path",
		ExpectedBehavior: []string{"states plainly that it has no real work to do"}, // no known command -> empty want
	}

	res := RunScenario(context.Background(), x, t.TempDir(), pluginRoot, skills.Skill{Name: "hello", Dir: skillDir}, s, "claude-code", "claude-sonnet-5")

	if res.Err != "" {
		t.Fatalf("RunScenario error: %s", res.Err)
	}
	if res.Skipped {
		t.Fatalf("RunScenario skipped: %s", res.SkipReason)
	}
	if !res.Pass {
		t.Errorf("Pass = false, want true: %+v", res)
	}
	if len(res.OrderWant) != 0 {
		t.Errorf("OrderWant = %v, want empty (no known command in expected_behavior)", res.OrderWant)
	}
}

func TestRunScenario_OrderMismatchFailsEvenWhenGraderPasses(t *testing.T) {
	pluginRoot := t.TempDir()
	skillDir := buildFixtureSkill(t, pluginRoot, "run-mutation", "happy_path")

	// The recorded fixture already carries an events.jsonl, as if a
	// prior (real) session had run mutation summarize before mutation
	// run — the wrong order relative to what expected_behavior demands
	// below.
	fixtureEvents := filepath.Join(skillDir, "evals", "fixtures", "happy_path", "events.jsonl")
	if err := os.WriteFile(fixtureEvents,
		[]byte(`{"at":"2026-01-01T00:00:00Z","type":"tool_run","command":"mutation summarize"}`+"\n"+
			`{"at":"2026-01-01T00:00:01Z","type":"tool_run","command":"mutation run"}`+"\n",
		), 0o600); err != nil {
		t.Fatal(err)
	}

	x := exectest.NewUnlisted(t).
		Then(exec.Output{Stdout: `{"result":"ran fine"}`, ExitCode: 0}).
		Then(passingGraderOutput())

	s := Scenario{
		Kind:             KindHappyPath,
		Query:            "run mutation for PROJ-1",
		Fixture:          "fixtures/happy_path",
		ExpectedBehavior: []string{"runs mutation run", "runs mutation summarize"},
	}

	res := RunScenario(context.Background(), x, t.TempDir(), pluginRoot, skills.Skill{Name: "run-mutation", Dir: skillDir}, s, "claude-code", "claude-sonnet-5")

	if res.Err != "" {
		t.Fatalf("RunScenario error: %s", res.Err)
	}
	if !res.GraderPass {
		t.Fatalf("grader should have passed: %+v", res)
	}
	if res.OrderOK {
		t.Errorf("OrderOK = true, want false: want %v got %v", res.OrderWant, res.OrderGot)
	}
	if res.Pass {
		t.Errorf("Pass = true, want false — a passing grade must not paper over a wrong-order events.jsonl")
	}
}

func TestRunScenario_CursorHostSkips(t *testing.T) {
	pluginRoot := t.TempDir()
	skillDir := buildFixtureSkill(t, pluginRoot, "hello", "happy_path")

	x := exectest.NewUnlisted(t) // no responses queued — cursorRunner must never call it

	s := Scenario{Kind: KindHappyPath, Query: "load the hello skill PROJ-1", Fixture: "fixtures/happy_path", ExpectedBehavior: []string{"x"}}
	res := RunScenario(context.Background(), x, t.TempDir(), pluginRoot, skills.Skill{Name: "hello", Dir: skillDir}, s, "cursor", "claude-sonnet-5")

	if !res.Skipped {
		t.Fatalf("expected Skipped, got %+v", res)
	}
}

func TestRunScenario_NoKeyInQuery(t *testing.T) {
	pluginRoot := t.TempDir()
	skillDir := buildFixtureSkill(t, pluginRoot, "hello", "happy_path")

	s := Scenario{Kind: KindHappyPath, Query: "load the hello skill", Fixture: "fixtures/happy_path", ExpectedBehavior: []string{"x"}}
	res := RunScenario(context.Background(), exectest.NewUnlisted(t), t.TempDir(), pluginRoot, skills.Skill{Name: "hello", Dir: skillDir}, s, "claude-code", "claude-sonnet-5")

	if res.Err == "" {
		t.Fatal("expected an error for a query with no ticket key")
	}
}

func TestRunScenario_UnknownHost(t *testing.T) {
	pluginRoot := t.TempDir()
	skillDir := buildFixtureSkill(t, pluginRoot, "hello", "happy_path")

	s := Scenario{Kind: KindHappyPath, Query: "load the hello skill PROJ-1", Fixture: "fixtures/happy_path", ExpectedBehavior: []string{"x"}}
	res := RunScenario(context.Background(), exectest.NewUnlisted(t), t.TempDir(), pluginRoot, skills.Skill{Name: "hello", Dir: skillDir}, s, "no-such-host", "claude-sonnet-5")

	if res.Err == "" {
		t.Fatal("expected an error for an unknown host")
	}
}
