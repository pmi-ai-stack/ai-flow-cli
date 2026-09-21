package handoff

import "slices"

// StageDef is one row of StageTable: everything status, skills lint
// (phase 10) and doctor (phase 8) need to know about a stage without
// hard-coding it themselves.
type StageDef struct {
	Stage Stage
	// Skill is the skill that drives this stage, or nil for a
	// CLI-only stage (fetch, locate, output).
	Skill *string
	// Command is the CLI-only stage's own driving command (without
	// the ticket key or the "ai-flow" prefix), or nil for a judgement
	// stage, which is driven by `stage start`/`stage complete` from
	// its skill instead.
	Command *string
	// GatePosition is this stage's gate position (see GatePositionFor).
	GatePosition GatePosition
	// SkipRule is the coded "Skip when" rule id (see ShouldRun).
	SkipRule string
	// Artifacts is every artifact the Workflow table names for this
	// stage, in the order it names them.
	Artifacts []string
}

func strp(s string) *string { return &s }

// stageTable is StageTable's backing data, built once from the spec's
// Workflow table.
var stageTable = []StageDef{
	{Stage: StageFetch, Skill: nil, Command: strp("ticket fetch"), GatePosition: GateNone, SkipRule: "never",
		Artifacts: []string{"ticket.json", "attachments/", "images.md"}},
	{Stage: StageLocate, Skill: nil, Command: strp("repo locate"), GatePosition: GateNone, SkipRule: "never",
		Artifacts: []string{"repo.json"}},
	{Stage: StagePlan, Skill: strp("plan-solution"), Command: nil, GatePosition: GatePost, SkipRule: "never",
		Artifacts: []string{"ops-targets.json", "standards.json", "guidance.md", "plan.md"}},
	{Stage: StageImplement, Skill: strp("implement-plan"), Command: nil, GatePosition: GateNone, SkipRule: "never",
		Artifacts: []string{"code", "tests", "implementation.log", "test-results.json"}},
	{Stage: StageRenderDrift, Skill: strp("assess-drift"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageRenderDrift),
		Artifacts: []string{"render/", "render-drift-results.json"}},
	{Stage: StageChecks, Skill: strp("apply-checks"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageChecks),
		Artifacts: []string{"checks-results.json"}},
	{Stage: StageDryRun, Skill: strp("dry-run-change"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageDryRun),
		Artifacts: []string{"plans/", "dryrun-results.json"}},
	{Stage: StageImpact, Skill: strp("assess-impact"), Command: nil, GatePosition: GatePost, SkipRule: skipRuleFor(StageImpact),
		Artifacts: []string{"impact-results.json", "impact.md"}},
	{Stage: StageRollback, Skill: strp("plan-rollback"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageRollback),
		Artifacts: []string{"rollback-results.json", "rollback-plan.md"}},
	{Stage: StageInfraTest, Skill: strp("test-infra"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageInfraTest),
		Artifacts: []string{"infra-test-results.json", "test files"}},
	{Stage: StageMutation, Skill: strp("run-mutation"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageMutation),
		Artifacts: []string{"mutation-results.json"}},
	{Stage: StageVisualTest, Skill: strp("run-visual-tests"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageVisualTest),
		Artifacts: []string{"visual-test-results.json", "screenshots/"}},
	{Stage: StageE2E, Skill: strp("run-e2e"), Command: nil, GatePosition: GateNone, SkipRule: skipRuleFor(StageE2E),
		Artifacts: []string{"e2e-results.json"}},
	{Stage: StageReview, Skill: strp("review-changes"), Command: nil, GatePosition: GateNone, SkipRule: "never",
		Artifacts: []string{"review.md"}},
	{Stage: StageOutput, Skill: nil, Command: strp("summary render"), GatePosition: GateNone, SkipRule: "never",
		Artifacts: []string{"output.md"}},
	{Stage: StageShip, Skill: strp("ship"), Command: nil, GatePosition: GatePre, SkipRule: "never",
		Artifacts: []string{"commit-message.md", "pr-body.md", "ship-results.json"}},
}

// StageTable returns every stage's definition, in pipeline order. The
// caller's to keep — always a fresh copy of the slice (the StageDef
// values themselves are immutable data, safe to share).
func StageTable() []StageDef {
	return slices.Clone(stageTable)
}

// StageTableEntry returns stage's StageDef and whether it was found.
func StageTableEntry(stage Stage) (StageDef, bool) {
	for _, d := range stageTable {
		if d.Stage == stage {
			return d, true
		}
	}
	return StageDef{}, false
}
