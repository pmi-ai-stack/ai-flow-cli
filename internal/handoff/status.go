package handoff

import "encoding/json"

// Approval is a stage entry's recorded gate decision: who approved it,
// when, and at which gate position. Only ever present on a stage whose
// GatePositionFor is pre or post — Gate is never "none".
type Approval struct {
	By   string       `json:"by"`
	At   string       `json:"at"`
	Gate GatePosition `json:"gate"`
}

// StageEntry is one row of status.json's stages array.
type StageEntry struct {
	Stage    Stage     `json:"stage"`
	State    State     `json:"state"`
	Approval *Approval `json:"approval"`
}

// Status is the full status.json shape: the state machine for one
// ticket. Stages is an array in pipeline order — order carries
// meaning, per 07-determinism.md.
type Status struct {
	Key    string       `json:"key"`
	Stages []StageEntry `json:"stages"`
}

// NewStatus builds a fresh Status for key: every stage pending, no
// approvals. This is what LoadStatus returns for a ticket whose
// status.json does not exist yet, and what `status KEY` reports for an
// empty handoff.
func NewStatus(key string) *Status {
	entries := make([]StageEntry, len(stages))
	for i, s := range stages {
		entries[i] = StageEntry{Stage: s, State: StatePending, Approval: nil}
	}
	return &Status{Key: key, Stages: entries}
}

// Entry returns a pointer to stage's entry, so callers mutate Status
// in place. Panics if stage is not one of Stages() — every caller in
// this codebase passes a compile-time-known constant or a value
// already checked with IsStage; a status.json naming an unknown stage
// is caught earlier, at decode/validate time, not here.
func (s *Status) Entry(stage Stage) *StageEntry {
	for i := range s.Stages {
		if s.Stages[i].Stage == stage {
			return &s.Stages[i]
		}
	}
	panic("handoff: status has no entry for stage " + string(stage))
}

// LoadStatus reads home's status.json for key, or returns a fresh
// NewStatus(key) when the file does not exist yet — a missing
// status.json is a normal state (no stage has run), never an error,
// so `status KEY` and every `stage` subcommand share one bootstrap
// path.
func LoadStatus(home, key string) (*Status, error) {
	data, err := ReadArtifact(StatusPath(home, key))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return NewStatus(key), nil
	}
	var st Status
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// SaveStatus writes s to home's status.json.
func SaveStatus(home string, s *Status) (string, error) {
	return WriteArtifact(TicketDir(home, s.Key), "status.json", "status", s)
}

// Next returns the first stage whose state is not pass, skipped or
// approved (the resume rule), and that stage's state. Both are nil
// when every stage is done.
func Next(s *Status) (stage *Stage, state *State) {
	for i := range s.Stages {
		e := &s.Stages[i]
		if e.State == StatePass || e.State == StateSkipped || e.State == StateApproved {
			continue
		}
		stg, stt := e.Stage, e.State
		return &stg, &stt
	}
	return nil, nil
}
