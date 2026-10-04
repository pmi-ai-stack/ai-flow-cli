package handoff

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// The four event types an events.jsonl line's own Type field carries —
// exported so a reader (internal/skills/evals's own call-order
// assertion, in particular) matches against the same constants these
// builders write, rather than a second, independently hand-typed copy
// of the same literal.
const (
	EventTypeStageTransition = "stage_transition"
	EventTypeSessionStart    = "session_start"
	EventTypeToolRun         = "tool_run"
	EventTypeGuardDecision   = "guard_decision"
	EventTypeStandardsSync   = "standards_sync"
	EventTypeUnlistedRun     = "unlisted_run"
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
	// RunID names the CLI invocation that wrote the event — the same
	// run_id every --verbose log line of that invocation carries, so a
	// stderr line can be joined to the record of what the run did. Empty
	// (and omitted) when the writer had no invocation at hand.
	RunID string `json:"run_id,omitempty"`
	// Host, HostVersion, PluginRoot and CLIVersion are set only on a
	// "session_start" event, written by `hook session-start` (phase 9).
	// doctor's doctorLastSessionHost (built in phase 8, unchanged since)
	// already tolerantly scans for Host — it starts reporting the moment
	// this field is populated, with no further change there.
	Host        string `json:"host,omitempty"`
	HostVersion string `json:"host_version,omitempty"`
	PluginRoot  string `json:"plugin_root,omitempty"`
	CLIVersion  string `json:"cli_version,omitempty"`
	// Allow is set only on a "guard_decision" event, written by
	// `hook guard` (phase 9) — Command carries the shell command or file
	// path the decision was about (GuardDecisionEvent's own doc comment).
	Allow *bool `json:"allow,omitempty"`
	// PackID, PackRef, PackRefKind and Synced are set only on a
	// "standards_sync" event, written by `standards sync` (phase 12).
	PackID      string `json:"pack_id,omitempty"`
	PackRef     string `json:"pack_ref,omitempty"`
	PackRefKind string `json:"pack_ref_kind,omitempty"`
	Synced      *bool  `json:"synced,omitempty"`
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
		Type:    EventTypeStageTransition,
		Stage:   string(stage),
		From:    string(from),
		To:      string(to),
		Command: command,
		Reason:  reason,
	}
}

// SessionStartEvent builds a "session_start" Event — `hook
// session-start`'s own write, appended to the global events.jsonl (a
// session start belongs to no ticket). Matches the Guardrails
// section's documented shape verbatim: "{ ts, host, host_version,
// plugin_root, cli_version }" (ts is At, set by AppendEvent).
func SessionStartEvent(host, hostVersion, pluginRoot, cliVersion string) Event {
	return Event{
		Type:        EventTypeSessionStart,
		Host:        host,
		HostVersion: hostVersion,
		PluginRoot:  pluginRoot,
		CLIVersion:  cliVersion,
	}
}

// ToolRunEvent builds a "tool_run" Event — the Handoff directory
// table's own words for events.jsonl ("this ticket's stage
// transitions, tool runs and guard decisions") name this category, but
// no phase before phase 11 ever wrote one: every ticket-scoped
// command's successful completion appends one from
// internal/cli/root.go's wrap, with command exactly the Catalog() name
// the command was invoked with, unless Catalog()'s own
// SkipToolRunEvent marks it as already covered by a richer event (see
// the phase 11 plan's design decision 1).
func ToolRunEvent(command string) Event {
	return Event{Type: EventTypeToolRun, Command: command}
}

// UnlistedRunEvent builds an "unlisted_run" Event: one process ai-flow
// started outside the Exec allowlist (the dev server and capture script
// `visual run` reads from the repository, the headless host CLI
// `skills evals` drives). Command is the full argv; AppendEvent redacts
// it like every other line.
func UnlistedRunEvent(argv []string) Event {
	return Event{Type: EventTypeUnlistedRun, Command: strings.Join(argv, " ")}
}

// GuardDecisionEvent builds a "guard_decision" Event — `hook guard`'s
// own write, appended to the ticket-scoped events.jsonl when the
// decision resolves to an active ticket, else the global one ("guard
// decisions whose path or cwd resolves to no active ticket" — the
// Handoff directory section's own words for what belongs in the
// global file). command carries the shell command or file path the
// decision was made about.
func GuardDecisionEvent(host, reason, command string, allow bool) Event {
	return Event{
		Type:    EventTypeGuardDecision,
		Host:    host,
		Reason:  reason,
		Command: command,
		Allow:   &allow,
	}
}

// StandardsSyncEvent builds a "standards_sync" Event — `standards
// sync`'s own write, appended to the global events.jsonl (a pack
// source belongs to no ticket). synced is false for a source Sync
// skipped (a pinned ref already cached, or a branch synced within the
// last day) — still worth a line, since "nothing changed" is itself
// diagnostic information doctor and a human both want.
func StandardsSyncEvent(id, ref, refKind string, synced bool) Event {
	return Event{
		Type:        EventTypeStandardsSync,
		PackID:      id,
		PackRef:     ref,
		PackRefKind: refKind,
		Synced:      &synced,
	}
}
