package handoff

import "testing"

func TestShouldRunNeverStagesAlwaysRun(t *testing.T) {
	never := []Stage{StageFetch, StageLocate, StagePlan, StageImplement, StageReview, StageOutput, StageShip}
	for _, stage := range never {
		run, rule := ShouldRun(stage, ShouldRunInput{})
		if !run {
			t.Errorf("ShouldRun(%s) run = false, want true", stage)
		}
		if rule != "never" {
			t.Errorf("ShouldRun(%s) rule = %q, want \"never\"", stage, rule)
		}
	}
}

// TestShouldRunStillDeferredStagesAlwaysRun pins phase 6's design
// decision 2, now covering the six skip rules still deferred after
// phase 7: they must keep answering run: true, and must do so
// regardless of what ShouldRunInput carries — a future phase adding
// its own fields to the struct must not accidentally change a rule it
// does not own yet.
func TestShouldRunStillDeferredStagesAlwaysRun(t *testing.T) {
	want := map[Stage]string{
		StageRenderDrift: "no_changed_target",
		StageChecks:      "no_check_applies",
		StageDryRun:      "no_changed_target_with_dry_run",
		StageImpact:      "dry_run_skipped",
		StageRollback:    "changeset_empty",
		StageInfraTest:   "no_changed_target_or_no_harness",
	}
	inputs := []ShouldRunInput{
		{},
		{HasFrontendFiles: true, HasAppFiles: true, HasE2ESuite: true, HasMutationTool: true},
		{HasFrontendFiles: false, HasAppFiles: true, HasE2ESuite: false, HasMutationTool: false},
	}
	for stage, rule := range want {
		for _, in := range inputs {
			run, gotRule := ShouldRun(stage, in)
			if !run {
				t.Errorf("ShouldRun(%s, %+v) run = false, want true (never silently skip before the rule is wired)", stage, in)
			}
			if gotRule != rule {
				t.Errorf("ShouldRun(%s, %+v) rule = %q, want %q", stage, in, gotRule, rule)
			}
		}
	}
}

// TestShouldRunVisualTest pins visual-test's real rule: no_frontend_files.
func TestShouldRunVisualTest(t *testing.T) {
	cases := []struct {
		name string
		in   ShouldRunInput
		want bool
	}{
		{"has frontend files", ShouldRunInput{HasFrontendFiles: true}, true},
		{"no frontend files", ShouldRunInput{HasFrontendFiles: false}, false},
		{"frontend files, irrelevant e2e fields set too", ShouldRunInput{HasFrontendFiles: true, HasAppFiles: true, HasE2ESuite: true}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run, rule := ShouldRun(StageVisualTest, c.in)
			if run != c.want {
				t.Errorf("ShouldRun(visual-test, %+v) run = %v, want %v", c.in, run, c.want)
			}
			if rule != "no_frontend_files" {
				t.Errorf("rule = %q, want no_frontend_files", rule)
			}
		})
	}
}

// TestShouldRunE2E pins e2e's real rule: no_app_files_or_no_e2e_suite —
// both HasAppFiles and HasE2ESuite must be true for the stage to run.
func TestShouldRunE2E(t *testing.T) {
	cases := []struct {
		name string
		in   ShouldRunInput
		want bool
	}{
		{"app files and suite", ShouldRunInput{HasAppFiles: true, HasE2ESuite: true}, true},
		{"app files, no suite", ShouldRunInput{HasAppFiles: true, HasE2ESuite: false}, false},
		{"no app files, suite present", ShouldRunInput{HasAppFiles: false, HasE2ESuite: true}, false},
		{"neither", ShouldRunInput{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run, rule := ShouldRun(StageE2E, c.in)
			if run != c.want {
				t.Errorf("ShouldRun(e2e, %+v) run = %v, want %v", c.in, run, c.want)
			}
			if rule != "no_app_files_or_no_e2e_suite" {
				t.Errorf("rule = %q, want no_app_files_or_no_e2e_suite", rule)
			}
		})
	}
}

// TestShouldRunMutation pins mutation's real rule (phase 7):
// no_app_files_or_no_mutation_tool — both HasAppFiles and
// HasMutationTool must be true for the stage to run.
func TestShouldRunMutation(t *testing.T) {
	cases := []struct {
		name string
		in   ShouldRunInput
		want bool
	}{
		{"app files and tool", ShouldRunInput{HasAppFiles: true, HasMutationTool: true}, true},
		{"app files, no tool", ShouldRunInput{HasAppFiles: true, HasMutationTool: false}, false},
		{"no app files, tool present", ShouldRunInput{HasAppFiles: false, HasMutationTool: true}, false},
		{"neither", ShouldRunInput{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run, rule := ShouldRun(StageMutation, c.in)
			if run != c.want {
				t.Errorf("ShouldRun(mutation, %+v) run = %v, want %v", c.in, run, c.want)
			}
			if rule != "no_app_files_or_no_mutation_tool" {
				t.Errorf("rule = %q, want no_app_files_or_no_mutation_tool", rule)
			}
		})
	}
}

// TestEveryStageHasASkipRuleAnswer proves ShouldRun and StageTable agree
// on every stage's coded rule id — a mismatch would mean a stage was
// renamed or added in one table and not the other.
func TestEveryStageHasASkipRuleAnswer(t *testing.T) {
	for _, stage := range Stages() {
		_, rule := ShouldRun(stage, ShouldRunInput{})
		def, ok := StageTableEntry(stage)
		if !ok {
			t.Fatalf("StageTableEntry(%s) not found", stage)
		}
		if def.SkipRule != rule {
			t.Errorf("stage %s: StageTable.SkipRule = %q, ShouldRun rule = %q — must agree", stage, def.SkipRule, rule)
		}
	}
}
