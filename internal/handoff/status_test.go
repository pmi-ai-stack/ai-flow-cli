package handoff

import "testing"

func TestNewStatus(t *testing.T) {
	s := NewStatus("PROJ-1")
	if s.Key != "PROJ-1" {
		t.Errorf("Key = %q, want PROJ-1", s.Key)
	}
	if len(s.Stages) != len(stages) {
		t.Fatalf("len(Stages) = %d, want %d", len(s.Stages), len(stages))
	}
	for i, e := range s.Stages {
		if e.Stage != stages[i] {
			t.Errorf("Stages[%d].Stage = %s, want %s", i, e.Stage, stages[i])
		}
		if e.State != StatePending {
			t.Errorf("Stages[%d].State = %s, want pending", i, e.State)
		}
		if e.Approval != nil {
			t.Errorf("Stages[%d].Approval = %+v, want nil", i, e.Approval)
		}
	}
}

func TestStatusEntry(t *testing.T) {
	s := NewStatus("PROJ-1")
	e := s.Entry(StagePlan)
	e.State = StateRunning
	if s.Entry(StagePlan).State != StateRunning {
		t.Error("Entry did not return a pointer into the slice — mutation did not stick")
	}
}

func TestStatusEntryPanicsOnUnknownStage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Entry(unknown stage) should panic")
		}
	}()
	NewStatus("PROJ-1").Entry(Stage("no-such-stage"))
}

func TestLoadStatusMissingFileReturnsFreshStatus(t *testing.T) {
	home := t.TempDir()
	s, err := LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if s.Key != "PROJ-1" || s.Entry(StageFetch).State != StatePending {
		t.Errorf("LoadStatus on a missing file should behave like NewStatus, got %+v", s)
	}
}

func TestSaveAndLoadStatusRoundTrip(t *testing.T) {
	home := t.TempDir()
	s := NewStatus("PROJ-1")
	s.Entry(StageFetch).State = StatePass
	if _, err := SaveStatus(home, s); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}

	got, err := LoadStatus(home, "PROJ-1")
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if got.Entry(StageFetch).State != StatePass {
		t.Errorf("round-tripped fetch state = %s, want pass", got.Entry(StageFetch).State)
	}
}

func TestNextOnFreshStatus(t *testing.T) {
	s := NewStatus("PROJ-1")
	stage, state := Next(s)
	if stage == nil || *stage != StageFetch {
		t.Errorf("Next stage = %v, want fetch", stage)
	}
	if state == nil || *state != StatePending {
		t.Errorf("Next state = %v, want pending", state)
	}
}

func TestNextSkipsPassSkippedApproved(t *testing.T) {
	s := NewStatus("PROJ-1")
	s.Entry(StageFetch).State = StatePass
	s.Entry(StageLocate).State = StateSkipped
	s.Entry(StagePlan).State = StateApproved
	s.Entry(StageImplement).State = StateRunning

	stage, state := Next(s)
	if stage == nil || *stage != StageImplement {
		t.Errorf("Next stage = %v, want implement", stage)
	}
	if state == nil || *state != StateRunning {
		t.Errorf("Next state = %v, want running", state)
	}
}

func TestNextWhenEveryStageIsDone(t *testing.T) {
	s := NewStatus("PROJ-1")
	for i := range s.Stages {
		s.Stages[i].State = StatePass
	}
	stage, state := Next(s)
	if stage != nil || state != nil {
		t.Errorf("Next = (%v, %v), want (nil, nil) when every stage is done", stage, state)
	}
}
