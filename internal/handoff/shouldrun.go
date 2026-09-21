package handoff

// skipRules names the coded "Skip when" rule for every stage that has
// a real one, from the Workflow table. The seven stages absent from
// this map (fetch, locate, plan, implement, review, output, ship) skip
// "never" — the rule is a fixed fact of the pipeline, not deferred.
//
// The other nine need data no phase before 6, 14, 18, 20, 23 or 24
// produces (ops detect's changed targets, detect mutation/frontend/e2e,
// dry-run's changeset). ShouldRun names the rule for all of them today
// so StageTable, skills lint (phase 10) and doctor have something to
// check skill/orchestrator text against, but always answers run: true
// until the owning phase wires the real evaluation — the only default
// that can never cause a stage to be silently skipped before its data
// exists.
var skipRules = map[Stage]string{
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

// skipRuleFor returns stage's coded skip-rule id: "never" for the
// seven stages whose rule really is never, else the id from
// skipRules.
func skipRuleFor(stage Stage) string {
	if rule, ok := skipRules[stage]; ok {
		return rule
	}
	return "never"
}

// ShouldRun implements `stage should-run KEY <stage>`: it is read-only
// and never mutates status.json. run is always true in phase 3 (see
// skipRules's doc comment above); rule is the coded rule id so
// callers — and doctor, once it exists — can tell which stages still
// have a real evaluator and which do not.
func ShouldRun(stage Stage) (run bool, rule string) {
	return true, skipRuleFor(stage)
}
