package handoff

import (
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// allStates is every state in the machine, used to prove a transition
// is refused from every state except its documented origins.
var allStates = []State{
	StatePending, StateRunning, StateAwaitingApproval, StatePass, StateFail, StateSkipped, StateApproved,
}

func freshEntry(stage Stage, state State) *Status {
	s := NewStatus("PROJ-1")
	s.Entry(stage).State = state
	return s
}

func wantStageNotAllowed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("err = %T(%v), want *envelope.Error", err, err)
	}
	if envErr.Code != "stage_not_allowed" {
		t.Errorf("err.Code = %q, want stage_not_allowed", envErr.Code)
	}
}

// --- Start ---

func TestStartFromPendingGateNoneGoesToRunning(t *testing.T) {
	s := freshEntry(StageImplement, StatePending)
	if err := Start(s, StageImplement, ShouldRunInput{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := s.Entry(StageImplement).State; got != StateRunning {
		t.Errorf("state = %s, want running", got)
	}
}

func TestStartFromPendingGatePostGoesToRunning(t *testing.T) {
	// A post-gate stage's gate fires at complete time, not start time —
	// starting it behaves exactly like a gate-none stage.
	s := freshEntry(StagePlan, StatePending)
	if err := Start(s, StagePlan, ShouldRunInput{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := s.Entry(StagePlan).State; got != StateRunning {
		t.Errorf("state = %s, want running", got)
	}
}

func TestStartFromPendingGatePreGoesToAwaitingApproval(t *testing.T) {
	s := freshEntry(StageShip, StatePending)
	if err := Start(s, StageShip, ShouldRunInput{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := s.Entry(StageShip).State; got != StateAwaitingApproval {
		t.Errorf("state = %s, want awaiting_approval", got)
	}
}

func TestStartFromAwaitingApprovalIsARevision(t *testing.T) {
	for _, stage := range []Stage{StagePlan, StageShip} { // post-gate and pre-gate
		s := freshEntry(stage, StateAwaitingApproval)
		if err := Start(s, stage, ShouldRunInput{}); err != nil {
			t.Fatalf("Start(%s): %v", stage, err)
		}
		if got := s.Entry(stage).State; got != StateRunning {
			t.Errorf("Start(%s) from awaiting_approval = %s, want running", stage, got)
		}
	}
}

func TestStartRefusedFromEveryOtherState(t *testing.T) {
	for _, st := range allStates {
		if st == StatePending || st == StateAwaitingApproval {
			continue
		}
		s := freshEntry(StageImplement, st)
		wantStageNotAllowed(t, Start(s, StageImplement, ShouldRunInput{}))
	}
}

// --- Complete ---

func TestCompleteFromRunningGateNoneGoesToPass(t *testing.T) {
	s := freshEntry(StageImplement, StateRunning)
	if err := Complete(s, StageImplement, false, GateInputs{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := s.Entry(StageImplement).State; got != StatePass {
		t.Errorf("state = %s, want pass", got)
	}
}

func TestCompleteWithFailGoesToFailRegardlessOfGate(t *testing.T) {
	for _, stage := range []Stage{StageImplement, StagePlan, StageShip} {
		s := freshEntry(stage, StateRunning)
		if err := Complete(s, stage, true, GateInputs{}); err != nil {
			t.Fatalf("Complete(%s, fail): %v", stage, err)
		}
		if got := s.Entry(stage).State; got != StateFail {
			t.Errorf("Complete(%s, fail) = %s, want fail", stage, got)
		}
	}
}

func TestCompletePlanAlwaysGatesToAwaitingApproval(t *testing.T) {
	s := freshEntry(StagePlan, StateRunning)
	if err := Complete(s, StagePlan, false, GateInputs{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := s.Entry(StagePlan).State; got != StateAwaitingApproval {
		t.Errorf("state = %s, want awaiting_approval", got)
	}
}

func TestCompleteImpactGatesOnlyWhenGateFires(t *testing.T) {
	notFiring := freshEntry(StageImpact, StateRunning)
	if err := Complete(notFiring, StageImpact, false, GateInputs{RiskGate: 40, Score: 10, Protected: false}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := notFiring.Entry(StageImpact).State; got != StatePass {
		t.Errorf("below the gate: state = %s, want pass", got)
	}

	firing := freshEntry(StageImpact, StateRunning)
	if err := Complete(firing, StageImpact, false, GateInputs{RiskGate: 40, Score: 40, Protected: false}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := firing.Entry(StageImpact).State; got != StateAwaitingApproval {
		t.Errorf("at the gate: state = %s, want awaiting_approval", got)
	}
}

func TestCompleteShipGoesToPass(t *testing.T) {
	// ship's gate is pre-position; Complete never re-gates it.
	s := freshEntry(StageShip, StateRunning)
	if err := Complete(s, StageShip, false, GateInputs{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := s.Entry(StageShip).State; got != StatePass {
		t.Errorf("state = %s, want pass", got)
	}
}

func TestCompleteRefusedFromEveryOtherState(t *testing.T) {
	for _, st := range allStates {
		if st == StateRunning {
			continue
		}
		s := freshEntry(StageImplement, st)
		wantStageNotAllowed(t, Complete(s, StageImplement, false, GateInputs{}))
	}
}

// --- Approve ---

func TestApprovePreGateGoesToRunningAndRecordsApproval(t *testing.T) {
	s := freshEntry(StageShip, StateAwaitingApproval)
	if err := Approve(s, StageShip, "alice", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	e := s.Entry(StageShip)
	if e.State != StateRunning {
		t.Errorf("state = %s, want running", e.State)
	}
	if e.Approval == nil || e.Approval.By != "alice" || e.Approval.Gate != GatePre {
		t.Errorf("Approval = %+v, want {alice, ..., pre}", e.Approval)
	}
}

func TestApprovePostGateGoesToApprovedAndRecordsApproval(t *testing.T) {
	s := freshEntry(StagePlan, StateAwaitingApproval)
	if err := Approve(s, StagePlan, "bob", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	e := s.Entry(StagePlan)
	if e.State != StateApproved {
		t.Errorf("state = %s, want approved", e.State)
	}
	if e.Approval == nil || e.Approval.By != "bob" || e.Approval.Gate != GatePost {
		t.Errorf("Approval = %+v, want {bob, ..., post}", e.Approval)
	}
}

func TestApproveRefusedFromEveryOtherState(t *testing.T) {
	for _, st := range allStates {
		if st == StateAwaitingApproval {
			continue
		}
		s := freshEntry(StagePlan, st)
		wantStageNotAllowed(t, Approve(s, StagePlan, "alice", "2026-01-01T00:00:00Z"))
	}
}

// --- Skip ---

func TestSkipFromPendingGoesToSkipped(t *testing.T) {
	s := freshEntry(StageLocate, StatePending)
	if err := Skip(s, StageLocate); err != nil {
		t.Fatalf("Skip: %v", err)
	}
	if got := s.Entry(StageLocate).State; got != StateSkipped {
		t.Errorf("state = %s, want skipped", got)
	}
}

func TestSkipRefusedFromEveryOtherState(t *testing.T) {
	for _, st := range allStates {
		if st == StatePending {
			continue
		}
		s := freshEntry(StageLocate, st)
		wantStageNotAllowed(t, Skip(s, StageLocate))
	}
}

// --- Reset ---

func TestResetFromEveryTerminalStateGoesToPending(t *testing.T) {
	for _, st := range []State{StateFail, StatePass, StateSkipped, StateApproved} {
		s := freshEntry(StageImplement, st)
		if err := Reset(s, StageImplement); err != nil {
			t.Fatalf("Reset from %s: %v", st, err)
		}
		if got := s.Entry(StageImplement).State; got != StatePending {
			t.Errorf("Reset from %s: state = %s, want pending", st, got)
		}
	}
}

func TestResetRefusedFromNonTerminalStates(t *testing.T) {
	for _, st := range []State{StatePending, StateRunning, StateAwaitingApproval} {
		s := freshEntry(StageImplement, st)
		wantStageNotAllowed(t, Reset(s, StageImplement))
	}
}

func TestResetCascadesToLaterStagesOnly(t *testing.T) {
	s := NewStatus("PROJ-1")
	// Drive fetch, locate and plan to done states with an approval on
	// plan, then run implement through render-drift.
	s.Entry(StageFetch).State = StatePass
	s.Entry(StageLocate).State = StatePass
	s.Entry(StagePlan).State = StateApproved
	s.Entry(StagePlan).Approval = &Approval{By: "alice", At: "2026-01-01T00:00:00Z", Gate: GatePost}
	s.Entry(StageImplement).State = StatePass
	s.Entry(StageRenderDrift).State = StateFail

	if err := Reset(s, StageImplement); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if s.Entry(StageFetch).State != StatePass {
		t.Error("fetch (earlier than implement) must be untouched")
	}
	if s.Entry(StageLocate).State != StatePass {
		t.Error("locate (earlier than implement) must be untouched")
	}
	if s.Entry(StagePlan).State != StateApproved || s.Entry(StagePlan).Approval == nil {
		t.Error("plan (earlier than implement) must keep its state and approval")
	}
	if s.Entry(StageImplement).State != StatePending {
		t.Error("implement itself must be reset to pending")
	}
	if s.Entry(StageRenderDrift).State != StatePending {
		t.Error("render-drift (later than implement) must be reset to pending")
	}
	// Every stage after render-drift was already pending; confirm the
	// cascade did not error out early or skip any of them.
	for _, stage := range Stages()[pipelineIndex(StageImplement):] {
		if s.Entry(stage).State != StatePending {
			t.Errorf("stage %s: state = %s, want pending", stage, s.Entry(stage).State)
		}
		if s.Entry(stage).Approval != nil {
			t.Errorf("stage %s: approval = %+v, want nil after reset", stage, s.Entry(stage).Approval)
		}
	}
}

func TestImplementOrLater(t *testing.T) {
	before := []Stage{StageFetch, StageLocate, StagePlan}
	fromOnward := []Stage{StageImplement, StageRenderDrift, StageShip}
	for _, stage := range before {
		if ImplementOrLater(stage) {
			t.Errorf("ImplementOrLater(%s) = true, want false", stage)
		}
	}
	for _, stage := range fromOnward {
		if !ImplementOrLater(stage) {
			t.Errorf("ImplementOrLater(%s) = false, want true", stage)
		}
	}
}
