package handoff

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// Event is one line of an events.jsonl file: the append-only
// wall-clock log of a ticket's stage transitions (and, from later
// phases, tool runs and guard decisions). Unlike every other artifact
// in this package, events.jsonl carries real time and is explicitly
// exempt from byte-stability — see 07-determinism.md's one named
// exception.
type Event struct {
	At      string `json:"at"`
	Type    string `json:"type"`
	Stage   string `json:"stage,omitempty"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Command string `json:"command,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// AppendEvent appends one compact JSON line to path, redacting any
// environment-variable secret it might contain first. At is set here,
// from time.Now() — the one call site in this package exempt from the
// injectable clock (.golangci.yml carries a matching forbidigo
// exclusion), since events.jsonl needs no reproducibility.
func AppendEvent(path string, e Event) error {
	e.At = time.Now().UTC().Format(time.RFC3339)

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("handoff: marshal event: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: path is always built from --home/a ticket key
	if err != nil {
		return fmt.Errorf("handoff: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	w := redact.New(os.Environ()).Writer(f)
	if _, err := w.Write(line); err != nil {
		return fmt.Errorf("handoff: append event to %s: %w", path, err)
	}
	return nil
}

// StageTransitionEvent builds a "stage_transition" Event for a
// successful stage command. reason is only ever non-empty for
// `stage skip`.
func StageTransitionEvent(stage Stage, from, to State, command, reason string) Event {
	return Event{
		Type:    "stage_transition",
		Stage:   string(stage),
		From:    string(from),
		To:      string(to),
		Command: command,
		Reason:  reason,
	}
}
