package handoff

import (
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// stageNotAllowed builds the exit-3 stage_not_allowed error every
// transition below returns when called from a state its own command
// does not accept.
func stageNotAllowed(cmd string, stage Stage, got State) error {
	return &envelope.Error{
		Code:    "stage_not_allowed",
		Message: fmt.Sprintf("stage %s %s: %s is %s", cmd, stage, stage, got),
	}
}

// Start implements `stage start KEY <stage>`. From pending: skipped
// when ShouldRun says so, else awaiting_approval for a pre-gate stage,
// else running. From awaiting_approval: always running — a revision,
// per "Resume and redo" ("stage start from awaiting_approval is
// reserved for the developer asking for changes"); the approval field
// is untouched since it is already nil. Any other starting state is
// refused. in is ShouldRun's own input — the zero value for every
// stage but visual-test and e2e (see ShouldRunInput's doc comment).
func Start(s *Status, stage Stage, in ShouldRunInput) error {
	e := s.Entry(stage)
	switch e.State {
	case StatePending:
		if run, _ := ShouldRun(stage, in); !run {
			e.State = StateSkipped
			return nil
		}
		if GatePositionFor(stage) == GatePre {
			e.State = StateAwaitingApproval
			return nil
		}
		e.State = StateRunning
		return nil
	case StateAwaitingApproval:
		e.State = StateRunning
		return nil
	default:
		return stageNotAllowed("start", stage, e.State)
	}
}

// GateInputs carries the values GateFires needs for a post-gate
// stage's completion. RiskGate/Score/Protected are only ever consulted
// for stage == impact (impact-results.json's score/protected and
// config.json's ops.risk_gate); the zero value is safe for every other
// stage, including plan, whose GateFires call ignores them entirely.
type GateInputs struct {
	RiskGate  int
	Score     int
	Protected bool
}

// Complete implements `stage complete KEY <stage> [--fail]`. Legal
// only from running. fail moves to fail; otherwise a post-gate stage
// whose GateFires call reports true moves to awaiting_approval, else
// pass. Complete stays a pure function — it is the caller's job (the
// internal/cli layer) to read config.json and impact-results.json and
// fill in gi before calling this.
func Complete(s *Status, stage Stage, fail bool, gi GateInputs) error {
	e := s.Entry(stage)
	if e.State != StateRunning {
		return stageNotAllowed("complete", stage, e.State)
	}
	switch {
	case fail:
		e.State = StateFail
	case GatePositionFor(stage) == GatePost && GateFires(stage, gi.RiskGate, gi.Score, gi.Protected):
		e.State = StateAwaitingApproval
	default:
		e.State = StatePass
	}
	return nil
}

// Approve implements `stage approve KEY <stage>`. Legal only from
// awaiting_approval. Records {by, at, gate} on the entry; a pre-gate
// stage moves to running (approved, now act); a post-gate stage moves
// to approved (acted, now approved).
func Approve(s *Status, stage Stage, by, at string) error {
	e := s.Entry(stage)
	if e.State != StateAwaitingApproval {
		return stageNotAllowed("approve", stage, e.State)
	}
	gate := GatePositionFor(stage)
	e.Approval = &Approval{By: by, At: at, Gate: gate}
	if gate == GatePre {
		e.State = StateRunning
		return nil
	}
	e.State = StateApproved
	return nil
}

// Skip implements `stage skip KEY <stage> --reason`. Legal only from
// pending. The reason is not stored on status.json (there is no slot
// for it in status.schema.json, and the spec never says there should
// be) — it is recorded on the events.jsonl entry the caller appends
// after a successful Skip.
func Skip(s *Status, stage Stage) error {
	e := s.Entry(stage)
	if e.State != StatePending {
		return stageNotAllowed("skip", stage, e.State)
	}
	e.State = StateSkipped
	return nil
}

// Reset implements `stage reset KEY <stage>`. Legal only when the
// stage's own state IsTerminal. Resets that stage and every later
// stage (pipeline order) to pending, clearing any recorded approval.
// It does not touch render/ or plans/ — the caller removes those
// directories itself when stage's pipeline position is implement or
// later (see ImplementOrLater), since that is a filesystem side effect
// this package's pure state-machine functions do not perform.
func Reset(s *Status, stage Stage) error {
	e := s.Entry(stage)
	if !IsTerminal(e.State) {
		return stageNotAllowed("reset", stage, e.State)
	}
	idx := pipelineIndex(stage)
	for i := range s.Stages {
		if pipelineIndex(s.Stages[i].Stage) >= idx {
			s.Stages[i].State = StatePending
			s.Stages[i].Approval = nil
		}
	}
	return nil
}

// ImplementOrLater reports whether stage's pipeline position is
// implement's or later — the rule for removing render/ and plans/ on
// reset ("Resume and redo": "from implement onward, removes render/
// and plans/"). Both directories are only ever populated starting
// phase 19, so this is a no-op through phase 18; os.RemoveAll on an
// absent path is not an error.
func ImplementOrLater(stage Stage) bool {
	return pipelineIndex(stage) >= pipelineIndex(StageImplement)
}
