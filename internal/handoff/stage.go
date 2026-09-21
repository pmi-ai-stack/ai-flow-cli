package handoff

import "slices"

// Stage is one of the sixteen stages in the Workflow table, in
// pipeline order. This type, and the order Stages returns, is the
// single source of truth status.schema.json's enum, NewStatus,
// StageTable and every skip-rule id derive from.
type Stage string

const (
	StageFetch       Stage = "fetch"
	StageLocate      Stage = "locate"
	StagePlan        Stage = "plan"
	StageImplement   Stage = "implement"
	StageRenderDrift Stage = "render-drift"
	StageChecks      Stage = "checks"
	StageDryRun      Stage = "dry-run"
	StageImpact      Stage = "impact"
	StageRollback    Stage = "rollback"
	StageInfraTest   Stage = "infra-test"
	StageMutation    Stage = "mutation"
	StageVisualTest  Stage = "visual-test"
	StageE2E         Stage = "e2e"
	StageReview      Stage = "review"
	StageOutput      Stage = "output"
	StageShip        Stage = "ship"
)

// stages is the pipeline in order. Every other package-level table in
// this package (StageTable, NewStatus's initial entries) is built by
// ranging over this slice, so there is exactly one place pipeline
// order is declared.
var stages = []Stage{
	StageFetch, StageLocate, StagePlan, StageImplement, StageRenderDrift,
	StageChecks, StageDryRun, StageImpact, StageRollback, StageInfraTest,
	StageMutation, StageVisualTest, StageE2E, StageReview, StageOutput, StageShip,
}

// Stages returns every stage in pipeline order. The caller's to keep —
// always a fresh copy.
func Stages() []Stage {
	return slices.Clone(stages)
}

// StageNames returns every stage's name in pipeline order, for the
// "one of: <list>" detail on a usage error — the same shape
// schema.Artifacts() already gives internal/cli/schema.go's own
// unknown-artifact error.
func StageNames() []string {
	names := make([]string, len(stages))
	for i, s := range stages {
		names[i] = string(s)
	}
	return names
}

// IsStage reports whether s is one of Stages().
func IsStage(s Stage) bool {
	for _, st := range stages {
		if st == s {
			return true
		}
	}
	return false
}

// pipelineIndex returns s's position in Stages(), or -1 if s is not a
// known stage. Used by Reset's cascade (every later stage) and by the
// "implement onward" rule for removing render/ and plans/.
func pipelineIndex(s Stage) int {
	for i, st := range stages {
		if st == s {
			return i
		}
	}
	return -1
}
