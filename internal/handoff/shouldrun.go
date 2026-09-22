package handoff

// skipRules names the coded "Skip when" rule for every stage that has
// a real one, from the Workflow table. The seven stages absent from
// this map (fetch, locate, plan, implement, review, output, ship) skip
// "never" — the rule is a fixed fact of the pipeline, not deferred.
//
// Phase 6 made two of the other nine real: visual-test and e2e (see
// ShouldRun below and phase 6's own design decision 1). Phase 7 makes
// a third: mutation. The remaining six need data no phase before 14,
// 18, 20, 23 or 24 produces (ops detect's changed targets and
// dry-run's changeset). Their entries in this map still name the coded
// rule id so StageTable, skills lint (phase 10) and doctor have
// something to check skill/orchestrator text against; ShouldRun's
// default branch keeps answering run: true for them regardless of what
// ShouldRunInput carries, until their owning phase adds a case for
// them explicitly — the only default that can never cause a stage to
// be silently skipped before its data exists.
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

// ShouldRunInput carries the data ShouldRun's two real rules need.
// Deliberately just three booleans today — internal/handoff imports no
// filesystem or internal/detect package, so the caller (internal/cli's
// stage.go) computes these and passes them in; a later phase adds
// fields the same way (HasChangedTarget for phase 18's rules, a
// changeset-derived field for phase 20's) without touching the two
// branches this phase already owns, since every field defaults to its
// zero value harmlessly for a stage that does not read it.
type ShouldRunInput struct {
	// HasFrontendFiles is true when the diff touches a recognizable
	// frontend file (internal/detect.Frontend over repo.json's
	// in_scope_files) — visual-test's own rule.
	HasFrontendFiles bool
	// HasAppFiles is true when internal/detect.AppFiles over
	// repo.json's in_scope_files is non-empty — e2e's rule (and, from
	// phase 7, mutation's).
	HasAppFiles bool
	// HasE2ESuite is true when internal/detect.E2E matched at least
	// one of repo.json's repositories — e2e's rule.
	HasE2ESuite bool
	// HasMutationTool is true when internal/detect.Mutation matched at
	// least one of repo.json's repositories — mutation's rule, phase 7.
	HasMutationTool bool
}

// ShouldRun implements `stage should-run KEY <stage>`: it is read-only
// and never mutates status.json. visual-test and e2e are real as of
// phase 6, mutation as of phase 7; every other stage ignores in
// entirely and answers run: true, exactly as it did before this type
// existed — a dedicated test pins that this stays true regardless of
// what in carries, so a future phase's own field additions cannot
// accidentally change a rule they don't own.
func ShouldRun(stage Stage, in ShouldRunInput) (run bool, rule string) {
	switch stage {
	case StageVisualTest:
		return in.HasFrontendFiles, skipRuleFor(stage)
	case StageE2E:
		return in.HasAppFiles && in.HasE2ESuite, skipRuleFor(stage)
	case StageMutation:
		return in.HasAppFiles && in.HasMutationTool, skipRuleFor(stage)
	default:
		return true, skipRuleFor(stage)
	}
}
