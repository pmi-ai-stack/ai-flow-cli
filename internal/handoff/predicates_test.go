package handoff

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile(%s): %v", path, err)
	}
}

func TestMustFindingsOpenMissingFile(t *testing.T) {
	dir := t.TempDir()
	open, err := MustFindingsOpen(dir)
	if err != nil {
		t.Fatalf("MustFindingsOpen: %v", err)
	}
	if open {
		t.Error("open = true, want false when checks-results.json does not exist")
	}
}

func TestMustFindingsOpenNoMustFindings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "checks-results.json"), `{"findings":[{"severity":"should","status":"open"}]}`)
	open, err := MustFindingsOpen(dir)
	if err != nil {
		t.Fatalf("MustFindingsOpen: %v", err)
	}
	if open {
		t.Error("open = true, want false — only severity should, not must")
	}
}

func TestMustFindingsOpenAnOpenMust(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "checks-results.json"), `{"findings":[
		{"severity":"must","status":"fixed"},
		{"severity":"must","status":"open"}
	]}`)
	open, err := MustFindingsOpen(dir)
	if err != nil {
		t.Fatalf("MustFindingsOpen: %v", err)
	}
	if !open {
		t.Error("open = false, want true — one finding is must+open")
	}
}

func TestMustFindingsOpenIgnoresStageTag(t *testing.T) {
	// Design decision 6: a must left open from the checks stage also
	// blocks ship, regardless of which stage recorded it.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "checks-results.json"), `{"findings":[{"severity":"must","status":"open","stage":"checks"}]}`)
	open, err := MustFindingsOpen(dir)
	if err != nil {
		t.Fatalf("MustFindingsOpen: %v", err)
	}
	if !open {
		t.Error("a must left open from checks must still be reported regardless of its stage tag")
	}
}

func TestOutputNotPassed(t *testing.T) {
	s := NewStatus("PROJ-1")
	if !OutputNotPassed(s) {
		t.Error("OutputNotPassed = false on a fresh status, want true")
	}
	s.Entry(StageOutput).State = StatePass
	if OutputNotPassed(s) {
		t.Error("OutputNotPassed = true after output passed, want false")
	}
}

func TestShipApproved(t *testing.T) {
	s := NewStatus("PROJ-1")
	if ShipApproved(s) {
		t.Error("ShipApproved = true on a fresh status, want false")
	}
	s.Entry(StageShip).Approval = &Approval{By: "alice", At: "2026-01-01T00:00:00Z", Gate: GatePre}
	if !ShipApproved(s) {
		t.Error("ShipApproved = false after recording an approval, want true")
	}
}

func TestPrimaryArtifactCoversEveryStage(t *testing.T) {
	for _, stage := range Stages() {
		if PrimaryArtifact(stage) == "" {
			t.Errorf("PrimaryArtifact(%s) is empty", stage)
		}
	}
}

func TestSchemaArtifactForJSONPrimaries(t *testing.T) {
	for _, stage := range Stages() {
		filename := PrimaryArtifact(stage)
		_, hasSchema := SchemaArtifactFor(filename)
		isJSON := filepath.Ext(filename) == ".json"
		if isJSON != hasSchema {
			t.Errorf("stage %s: primary artifact %s: hasSchema=%v, isJSON=%v — every JSON primary artifact must be schema-registered and vice versa", stage, filename, hasSchema, isJSON)
		}
	}
}
