package handoff

import "testing"

func TestStagesOrder(t *testing.T) {
	want := []Stage{
		StageFetch, StageLocate, StagePlan, StageImplement, StageRenderDrift,
		StageChecks, StageDryRun, StageImpact, StageRollback, StageInfraTest,
		StageMutation, StageVisualTest, StageE2E, StageReview, StageOutput, StageShip,
	}
	got := Stages()
	if len(got) != len(want) {
		t.Fatalf("len(Stages()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Stages()[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestStagesReturnsACopy(t *testing.T) {
	a := Stages()
	a[0] = "mutated"
	b := Stages()
	if b[0] != StageFetch {
		t.Error("Stages() did not return a fresh copy — mutation leaked into the package-level slice")
	}
}

func TestIsStage(t *testing.T) {
	if !IsStage(StageShip) {
		t.Error("IsStage(StageShip) = false, want true")
	}
	if IsStage(Stage("no-such-stage")) {
		t.Error("IsStage(unknown) = true, want false")
	}
}

func TestPipelineIndex(t *testing.T) {
	if pipelineIndex(StageFetch) != 0 {
		t.Errorf("pipelineIndex(fetch) = %d, want 0", pipelineIndex(StageFetch))
	}
	if pipelineIndex(StageShip) != len(stages)-1 {
		t.Errorf("pipelineIndex(ship) = %d, want %d", pipelineIndex(StageShip), len(stages)-1)
	}
	if pipelineIndex(Stage("no-such-stage")) != -1 {
		t.Error("pipelineIndex(unknown) should be -1")
	}
}
