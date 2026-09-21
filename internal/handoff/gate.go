package handoff

// GatePosition is where a stage's human gate sits, if it has one.
type GatePosition string

const (
	// GateNone: the stage has no gate at all.
	GateNone GatePosition = "none"
	// GatePre: the gate comes before the stage's own work — ship only.
	// stage start always lands in awaiting_approval; stage approve moves
	// it to running so the work (commit, push, pr create) happens after
	// approval.
	GatePre GatePosition = "pre"
	// GatePost: the gate comes after the stage's own work, and only
	// when GateFires says so — plan and impact.
	GatePost GatePosition = "post"
)

// GatePositionFor returns stage's gate position.
func GatePositionFor(stage Stage) GatePosition {
	switch stage {
	case StageShip:
		return GatePre
	case StagePlan, StageImpact:
		return GatePost
	default:
		return GateNone
	}
}

// GateFires decides whether a post-gate stage's completion opens the
// gate. It is only ever meaningful for GatePost stages: plan always
// fires (there is always a plan to approve); impact fires when the
// score meets ops.risk_gate or any action is protected — the two
// documented gate triggers. Called with any other stage, it returns
// false (there is no post-gate to fire).
//
// riskGate, score and protected come from config.json's ops.risk_gate
// and the ticket's impact-results.json respectively; this function
// takes them as plain parameters rather than reading either file
// itself, so it stays a pure function the state-machine tests can
// drive directly, and so it is already the exact hook phase 23's real
// `ops impact` result satisfies (see the plan's design decision 10).
func GateFires(stage Stage, riskGate int, score int, protected bool) bool {
	switch stage {
	case StagePlan:
		return true
	case StageImpact:
		return score >= riskGate || protected
	default:
		return false
	}
}
