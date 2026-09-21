package handoff

import "testing"

func TestGatePositionFor(t *testing.T) {
	cases := map[Stage]GatePosition{
		StageShip:   GatePre,
		StagePlan:   GatePost,
		StageImpact: GatePost,
		StageFetch:  GateNone,
		StageReview: GateNone,
		StageOutput: GateNone,
	}
	for stage, want := range cases {
		if got := GatePositionFor(stage); got != want {
			t.Errorf("GatePositionFor(%s) = %s, want %s", stage, got, want)
		}
	}
}

func TestGateFiresPlanAlwaysFires(t *testing.T) {
	if !GateFires(StagePlan, 0, 0, false) {
		t.Error("plan must always fire, regardless of riskGate/score/protected")
	}
	if !GateFires(StagePlan, 100, 0, false) {
		t.Error("plan must always fire")
	}
}

func TestGateFiresImpactBoundary(t *testing.T) {
	const riskGate = 40
	if GateFires(StageImpact, riskGate, riskGate-1, false) {
		t.Errorf("GateFires(impact, riskGate=%d, score=%d, protected=false) = true, want false (below the gate)", riskGate, riskGate-1)
	}
	if !GateFires(StageImpact, riskGate, riskGate, false) {
		t.Errorf("GateFires(impact, riskGate=%d, score=%d, protected=false) = false, want true (score == risk_gate fires)", riskGate, riskGate)
	}
	if !GateFires(StageImpact, riskGate, riskGate+1, false) {
		t.Error("a score above risk_gate must fire")
	}
	if !GateFires(StageImpact, riskGate, 0, true) {
		t.Error("protected must fire the gate regardless of score")
	}
}

func TestGateFiresEverythingElseNeverFires(t *testing.T) {
	for _, stage := range Stages() {
		if stage == StagePlan || stage == StageImpact {
			continue
		}
		if GateFires(stage, 0, 100, true) {
			t.Errorf("GateFires(%s, ...) = true, want false — only plan and impact ever fire", stage)
		}
	}
}
