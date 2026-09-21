package handoff

import "testing"

func TestShouldRunNeverStagesAlwaysRun(t *testing.T) {
	never := []Stage{StageFetch, StageLocate, StagePlan, StageImplement, StageReview, StageOutput, StageShip}
	for _, stage := range never {
		run, rule := ShouldRun(stage)
		if !run {
			t.Errorf("ShouldRun(%s) run = false, want true", stage)
		}
		if rule != "never" {
			t.Errorf("ShouldRun(%s) rule = %q, want \"never\"", stage, rule)
		}
	}
}

func TestShouldRunDeferredStagesRunWithCodedRule(t *testing.T) {
	want := map[Stage]string{
		StageRenderDrift: "no_changed_target",
		StageChecks:      "no_check_applies",
		StageDryRun:      "no_changed_target_with_dry_run",
		StageImpact:      "dry_run_skipped",
		StageRollback:    "changeset_empty",
		StageInfraTest:   "no_changed_target_or_no_harness",
		StageMutation:    "no_app_files_or_no_mutation_tool",
		StageVisualTest:  "no_frontend_files",
		StageE2E:         "no_app_files_or_no_e2e_suite",
	}
	for stage, rule := range want {
		run, gotRule := ShouldRun(stage)
		if !run {
			t.Errorf("ShouldRun(%s) run = false, want true (never silently skip before the rule is wired)", stage)
		}
		if gotRule != rule {
			t.Errorf("ShouldRun(%s) rule = %q, want %q", stage, gotRule, rule)
		}
	}
}

// TestEveryStageHasASkipRuleAnswer proves ShouldRun and StageTable agree
// on every stage's coded rule id — a mismatch would mean a stage was
// renamed or added in one table and not the other.
func TestEveryStageHasASkipRuleAnswer(t *testing.T) {
	for _, stage := range Stages() {
		_, rule := ShouldRun(stage)
		def, ok := StageTableEntry(stage)
		if !ok {
			t.Fatalf("StageTableEntry(%s) not found", stage)
		}
		if def.SkipRule != rule {
			t.Errorf("stage %s: StageTable.SkipRule = %q, ShouldRun rule = %q — must agree", stage, def.SkipRule, rule)
		}
	}
}
