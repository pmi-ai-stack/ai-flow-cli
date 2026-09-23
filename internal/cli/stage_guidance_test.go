package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readGuidance(t *testing.T, home, key string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, key, "guidance.md"))
	if err != nil {
		t.Fatalf("ReadFile guidance.md: %v", err)
	}
	return string(data)
}

// TestStageStartWritesGuidance proves `stage start`'s standards
// integration: the stage's guidance.md and the ticket's standards.json
// both land, and the guidance carries the repository's own
// instructions file verbatim as a `must`.
func TestStageStartWritesGuidance(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")
	runStage(t, home, 0, "start", "PROJ-1", "implement")

	if _, err := os.Stat(filepath.Join(home, "PROJ-1", "standards.json")); err != nil {
		t.Fatalf("stage start wrote no standards.json: %v", err)
	}

	got := readGuidance(t, home, "PROJ-1")
	for _, want := range []string{
		"# Guidance — PROJ-1 · implement\n",
		"Packs: reference@embedded (embedded)",
		"Languages: go\n",
		"## [must] repo.instructions\n",
		"Wrap every error with %w.",
		"Dropped by budget: ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("guidance.md is missing %q:\n%s", want, firstLines(got, 12))
		}
	}
}

// TestStageStartIdempotentBytes is this phase's own Done when:
// "`stage start KEY implement` writes a guidance.md whose content is a
// pure function of the resolution inputs, and re-running it writes
// identical bytes."
func TestStageStartIdempotentBytes(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")

	runStage(t, home, 0, "start", "PROJ-1", "implement")
	first := readGuidance(t, home, "PROJ-1")

	// A second start from `running` is refused by the state machine, so
	// complete and reset back to pending, then start again — the re-run
	// the Done when describes, through the same command path.
	// `stage complete implement` validates its primary artifact, hence
	// the stub test-results.json.
	writeFixture(t, home, "PROJ-1", "test-results", "test-results.json")
	runStage(t, home, 0, "complete", "PROJ-1", "implement")
	runStage(t, home, 0, "reset", "PROJ-1", "implement")
	runStage(t, home, 0, "start", "PROJ-1", "implement")
	second := readGuidance(t, home, "PROJ-1")

	if first != second {
		t.Errorf("re-running stage start wrote different bytes:\n--- first ---\n%s\n--- second ---\n%s",
			firstLines(first, 12), firstLines(second, 12))
	}
}

// TestStageStartRewritesGuidancePerStage proves the per-stage half of
// the refresh rule: starting review after implement rewrites
// guidance.md even though the resolution hash did not move.
func TestStageStartRewritesGuidancePerStage(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")

	runStage(t, home, 0, "start", "PROJ-1", "implement")
	implement := readGuidance(t, home, "PROJ-1")

	runStage(t, home, 0, "start", "PROJ-1", "review")
	review := readGuidance(t, home, "PROJ-1")

	if !strings.Contains(implement, "· implement\n") {
		t.Error("the implement guidance does not name its own stage")
	}
	if !strings.Contains(review, "· review\n") {
		t.Error("guidance.md was not rewritten when the review stage started")
	}
}

// TestStageStartSkipsResolutionBeforeLocate proves the documented
// tolerance: the stage machine does not enforce pipeline order, so a
// plan start before repo.json exists must not fail — it simply resolves
// nothing.
func TestStageStartSkipsResolutionBeforeLocate(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	runStage(t, home, 0, "start", "PROJ-1", "fetch")
	runStage(t, home, 0, "start", "PROJ-1", "plan")

	for _, name := range []string{"standards.json", "guidance.md"} {
		if _, err := os.Stat(filepath.Join(home, "PROJ-1", name)); err == nil {
			t.Errorf("%s was written before repo.json exists", name)
		}
	}
}

// TestStageStartRefusesMissingRepoInstructions proves the loader rule
// reaches the command: a repository with neither AGENTS.md nor
// CLAUDE.md is exit 4, not a silently-empty guidance.md.
func TestStageStartRefusesMissingRepoInstructions(t *testing.T) {
	home, repoDir := standardsTicket(t, "PROJ-1")
	if err := os.Remove(filepath.Join(repoDir, "AGENTS.md")); err != nil {
		t.Fatalf("Remove AGENTS.md: %v", err)
	}

	stdout, _, ec := runRoot(t, "stage", "start", "PROJ-1", "implement", "--home", home)
	if ec != 4 {
		t.Fatalf("exitCode = %d, want 4\n%s", ec, stdout.String())
	}
	var env struct {
		Error struct {
			Code    string   `json:"code"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
	if env.Error.Code != "missing_repo_instructions" {
		t.Errorf("error.code = %q, want missing_repo_instructions", env.Error.Code)
	}
	if len(env.Error.Details) != 1 || !strings.Contains(env.Error.Details[0], "acme.billing.api") {
		t.Errorf("details = %v, want one entry naming the repository", env.Error.Details)
	}
}

// TestDoctorReportsBudgetOverflow proves the doctor line this phase
// owns: a `must` set that alone exceeds guidance_budget_tokens is
// reported, in the result and as a top-level warning, with nothing
// truncated.
func TestDoctorReportsBudgetOverflow(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")
	shrinkBudget(t, home, 10)

	runStage(t, home, 0, "start", "PROJ-1", "implement")

	stdout, _, ec := runRoot(t, "doctor", "--home", home)
	if ec != 0 {
		t.Fatalf("doctor: exitCode = %d\n%s", ec, stdout.String())
	}
	var env struct {
		Result struct {
			GuidanceBudget *struct {
				Key    string `json:"key"`
				Tokens int    `json:"tokens"`
				Stages []struct {
					Stage             string `json:"stage"`
					MustOverflowBytes int    `json:"must_overflow_bytes"`
				} `json:"stages"`
			} `json:"guidance_budget"`
		} `json:"result"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}

	gb := env.Result.GuidanceBudget
	if gb == nil {
		t.Fatal("doctor reports no guidance_budget block for the active ticket")
	}
	if gb.Key != "PROJ-1" || gb.Tokens != 10 {
		t.Errorf("guidance_budget = %+v, want key PROJ-1 and tokens 10", gb)
	}
	if len(gb.Stages) == 0 {
		t.Fatal("guidance_budget.stages is empty, but a 10-token budget cannot hold the must set")
	}
	for _, s := range gb.Stages {
		if s.MustOverflowBytes <= 0 {
			t.Errorf("stage %s reports must_overflow_bytes = %d, want > 0", s.Stage, s.MustOverflowBytes)
		}
	}

	var warned bool
	for _, w := range env.Warnings {
		if strings.Contains(w, "must sections exceed guidance_budget_tokens") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want one naming the must-section overflow", env.Warnings)
	}

	// Nothing is truncated: every must is still in guidance.md.
	if !strings.Contains(readGuidance(t, home, "PROJ-1"), "## [must] repo.instructions") {
		t.Error("an overflowing budget dropped a must section; nothing may truncate a must")
	}
}

// shrinkBudget rewrites config.json's guidance_budget_tokens in place,
// the one knob these tests need that `config init`'s template does not
// expose as a flag.
func shrinkBudget(t *testing.T, home string, tokens int) {
	t.Helper()
	path := filepath.Join(home, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile config.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode config.json: %v", err)
	}
	standards, ok := cfg["standards"].(map[string]any)
	if !ok {
		t.Fatal("config.json has no standards block")
	}
	standards["guidance_budget_tokens"] = tokens

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("encode config.json: %v", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("WriteFile config.json: %v", err)
	}
}

// firstLines trims a long artifact to its first n lines, so a failing
// assertion prints a usable excerpt instead of 23 kB of guidance.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}
