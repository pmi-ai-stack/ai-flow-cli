package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readEventTypes reads path (a ticket's events.jsonl) and returns each
// line's (type, command) pair, in file order — this package's own
// black-box check that internal/handoff.ToolRunEvent (phase 11) is
// actually reaching the file through wrap, not just internal/handoff's
// own unit tests of AppendEvent/ToolRunEvent in isolation.
func readEventTypes(t *testing.T, path string) []struct{ Type, Command string } {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var got []struct{ Type, Command string }
	for line := range bytes.SplitSeq(bytes.TrimRight(data, "\n"), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var e struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("events.jsonl line is not valid JSON: %v\n%s", err, line)
		}
		got = append(got, struct{ Type, Command string }{e.Type, e.Command})
	}
	return got
}

// TestWrapAppendsToolRunEvent proves internal/cli/root.go's wrap
// (phase 11) appends exactly one "tool_run" event per successful,
// non-simulate, ticket-scoped, non-"stage "-prefixed command — the
// central mechanism internal/skills/evals's own order assertion reads
// back (see the phase 11 plan's design decision 1).
func TestWrapAppendsToolRunEvent(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if err := os.MkdirAll(filepath.Join(home, key), 0o700); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(home, key, "events.jsonl")

	if _, _, exitCode := runRoot(t, "status", key, "--home", home); exitCode != 0 {
		t.Fatalf("status: exitCode = %d", exitCode)
	}
	if _, _, exitCode := runRoot(t, "status", key, "--home", home); exitCode != 0 {
		t.Fatalf("status: exitCode = %d", exitCode)
	}

	got := readEventTypes(t, eventsPath)
	if len(got) != 2 {
		t.Fatalf("got %d event(s), want exactly 2:\n%+v", len(got), got)
	}
	for _, e := range got {
		if e.Type != "tool_run" || e.Command != "status" {
			t.Errorf("event = %+v, want {tool_run, status}", e)
		}
	}
}

// TestWrapDoesNotAppendToolRunEventUnderSimulate proves --simulate
// suppresses the tool_run write too, not just the command's own real
// side effects — an event recording something that did not actually
// happen would be worse than no event at all.
func TestWrapDoesNotAppendToolRunEventUnderSimulate(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if err := os.MkdirAll(filepath.Join(home, key), 0o700); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(home, key, "events.jsonl")

	if _, _, exitCode := runRoot(t, "status", key, "--home", home, "--simulate"); exitCode != 0 {
		t.Fatalf("status --simulate: exitCode = %d", exitCode)
	}

	if got := readEventTypes(t, eventsPath); len(got) != 0 {
		t.Errorf("got %d event(s) under --simulate, want 0: %+v", len(got), got)
	}
}

// TestWrapDoesNotDoubleLogStageCommands proves a "stage "-prefixed
// command's own StageTransitionEvent (written by stage.go itself) is
// not joined by a redundant tool_run event for the identical call —
// design decision 1's own reasoning made concrete: stage should-run is
// ticket-scoped, prefixed with "stage ", and writes nothing at all (it
// is documented read-only), so it is the simplest command that proves
// wrap's own exclusion fires without needing a full stage-start
// fixture.
func TestWrapDoesNotDoubleLogStageCommands(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if err := os.MkdirAll(filepath.Join(home, key), 0o700); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(home, key, "events.jsonl")

	if _, _, exitCode := runRoot(t, "stage", "should-run", key, "fetch", "--home", home); exitCode != 0 {
		t.Fatalf("stage should-run: exitCode = %d", exitCode)
	}

	if got := readEventTypes(t, eventsPath); len(got) != 0 {
		t.Errorf("got %d event(s) for a stage-prefixed, read-only command, want 0: %+v", len(got), got)
	}
}
