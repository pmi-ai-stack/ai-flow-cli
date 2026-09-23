package handoff

import (
	"slices"
	"testing"
)

func TestStageTableCoversEveryStageInOrder(t *testing.T) {
	tbl := StageTable()
	if len(tbl) != len(stages) {
		t.Fatalf("len(StageTable()) = %d, want %d", len(tbl), len(stages))
	}
	for i, def := range tbl {
		if def.Stage != stages[i] {
			t.Errorf("StageTable()[%d].Stage = %s, want %s", i, def.Stage, stages[i])
		}
	}
}

func TestStageTableCLIOnlyStagesHaveNoSkillButACommand(t *testing.T) {
	cliOnly := map[Stage]string{
		StageFetch:  "ticket fetch",
		StageLocate: "repo locate",
		StageOutput: "summary render",
	}
	for stage, wantCmd := range cliOnly {
		def, ok := StageTableEntry(stage)
		if !ok {
			t.Fatalf("StageTableEntry(%s) not found", stage)
		}
		if def.Skill != nil {
			t.Errorf("%s: Skill = %v, want nil (CLI-only)", stage, *def.Skill)
		}
		if def.Command == nil || *def.Command != wantCmd {
			t.Errorf("%s: Command = %v, want %q", stage, def.Command, wantCmd)
		}
	}
}

func TestStageTableJudgementStagesHaveASkillNoCommand(t *testing.T) {
	for _, stage := range Stages() {
		if stage == StageFetch || stage == StageLocate || stage == StageOutput {
			continue
		}
		def, ok := StageTableEntry(stage)
		if !ok {
			t.Fatalf("StageTableEntry(%s) not found", stage)
		}
		if def.Skill == nil {
			t.Errorf("%s: Skill = nil, want a skill name", stage)
		}
		if def.Command != nil {
			t.Errorf("%s: Command = %v, want nil (judgement stages are driven by stage start/complete)", stage, *def.Command)
		}
	}
}

func TestStageTableGatePositionsMatchGatePositionFor(t *testing.T) {
	for _, stage := range Stages() {
		def, _ := StageTableEntry(stage)
		if def.GatePosition != GatePositionFor(stage) {
			t.Errorf("%s: StageTable.GatePosition = %s, GatePositionFor = %s — must agree", stage, def.GatePosition, GatePositionFor(stage))
		}
	}
}

func TestStageTableEntryUnknownStage(t *testing.T) {
	if _, ok := StageTableEntry(Stage("no-such-stage")); ok {
		t.Error("StageTableEntry(unknown) = ok, want not found")
	}
}

// TestGuidanceFirstAppearsAtPlan makes PlanOrLater's own doc comment
// executable: it claims the plan stage is where guidance.md first
// appears in this table, and PlanOrLater hard-codes StagePlan to match.
// If a future phase moves guidance.md earlier, the predicate and its
// justification would silently disagree — this catches that.
func TestGuidanceFirstAppearsAtPlan(t *testing.T) {
	for _, d := range StageTable() {
		if slices.Contains(d.Artifacts, "guidance.md") {
			if d.Stage != StagePlan {
				t.Errorf("guidance.md first appears at stage %s, but PlanOrLater hard-codes %s", d.Stage, StagePlan)
			}
			return
		}
	}
	t.Error("no stage in StageTable() names guidance.md as an artifact")
}
