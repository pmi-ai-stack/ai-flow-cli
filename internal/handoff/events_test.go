package handoff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendEventWritesOneJSONLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	if err := AppendEvent(path, StageTransitionEvent(StageFetch, StatePending, StateRunning, "stage start", "")); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if err := AppendEvent(path, StageTransitionEvent(StageFetch, StateRunning, StatePass, "stage complete", "")); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), data)
	}
	var e Event
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("line 1 is not valid JSON: %v", err)
	}
	if e.Type != "stage_transition" || e.Stage != "fetch" || e.From != "pending" || e.To != "running" {
		t.Errorf("event = %+v, want the fetch pending->running transition", e)
	}
	if e.At == "" {
		t.Error("At must be set")
	}
}

func TestAppendEventRecordsSkipReason(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := AppendEvent(path, StageTransitionEvent(StageLocate, StatePending, StateSkipped, "stage skip", "not applicable to this ticket")); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	data, _ := os.ReadFile(path)
	var e Event
	if err := json.Unmarshal(data[:len(data)-1], &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.Reason != "not applicable to this ticket" {
		t.Errorf("Reason = %q, want the skip reason", e.Reason)
	}
}

func TestAppendEventRedactsSecrets(t *testing.T) {
	t.Setenv("AI_FLOW_JIRA_TOKEN", "super-secret-token-value")
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// A command field carrying a secret-shaped value should never reach
	// the file unredacted.
	e := StageTransitionEvent(StageFetch, StatePending, StateRunning, "stage start super-secret-token-value", "")
	if err := AppendEvent(path, e); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "super-secret-token-value") {
		t.Errorf("events.jsonl contains the raw secret value:\n%s", data)
	}
	if !strings.Contains(string(data), "<redacted:") {
		t.Errorf("events.jsonl does not contain a redaction token:\n%s", data)
	}
}
