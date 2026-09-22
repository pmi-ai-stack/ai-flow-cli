// Package agenthost is the CLI's one boundary with the two agent
// hosts (Claude Code, Cursor): the normalized types the guard and the
// hook commands operate on, and the Adapter interface each host's own
// package (internal/agenthost/claudecode, internal/agenthost/cursor)
// implements against its own wire shape. There is no shared JSON
// payload between hosts — only this shared Go type, which each
// Adapter's ParseHook builds from its own host's own fields.
package agenthost

import (
	"fmt"
	"strings"
)

// Host names one of the two supported agent hosts.
type Host string

const (
	HostClaudeCode Host = "claude-code"
	HostCursor     Host = "cursor"
)

// EventKind is what a normalized HookEvent represents.
type EventKind string

const (
	// EventShell is a PreToolUse call to a shell-running tool (Claude
	// Code's Bash, Cursor's Shell).
	EventShell EventKind = "shell"
	// EventWrite is a PreToolUse call to a file-editing tool (Claude
	// Code's Write/Edit, Cursor's Write).
	EventWrite EventKind = "write"
	// EventSessionStart is a SessionStart/sessionStart call.
	EventSessionStart EventKind = "session_start"
	// EventUnknown is a payload whose JSON parsed but whose event or
	// tool name this adapter does not recognize — the Guardrails
	// section's "hook_shape_unknown": always allowed, never denied.
	EventUnknown EventKind = "unknown"
)

// HookEvent is the normalized shape every Adapter's ParseHook builds,
// and the one shape guard.Evaluate reads — the Guardrails section's
// own words: "normalizes it to { host, event, command, file_path,
// cwd }". PluginRoot and HostVersion are populated only for
// EventSessionStart.
type HookEvent struct {
	Host        Host
	Event       EventKind
	Command     string
	FilePath    string
	Cwd         string
	PluginRoot  string
	HostVersion string
}

// Decision is what guard.Evaluate returns and what EncodeHook turns
// into a host-shaped response. OwningCommand names the ai-flow command
// that performs the equivalent operation, folded into UserMessage/
// AgentMessage by the caller that builds a Decision, not by
// EncodeHook, so every host prints the same words. Reason is a short,
// stable, machine-facing code (e.g. "hook_shape_unknown",
// "row6_git_publish") — never shown to a human, only ever written to
// events.jsonl by the hook guard command.
type Decision struct {
	Allow         bool
	Reason        string
	UserMessage   string
	AgentMessage  string
	OwningCommand string
}

// Detection is one Adapter's Detect result: whether that host's plugin
// was found on this machine, its manifest version and its plugin root.
type Detection struct {
	Version    string
	PluginRoot string
}

// Adapter is what internal/agenthost/claudecode and
// internal/agenthost/cursor each implement — one per host, never a
// third generic implementation.
type Adapter interface {
	// Host returns this adapter's Host constant.
	Host() Host
	// Detect looks for this host's plugin installed under home (the
	// resolved --home directory, since plugin skill/hook roots are
	// documented relative to it) and returns its manifest version and
	// plugin root when found. A real filesystem error is returned as
	// err; "not installed here" is (Detection{}, false, nil) — Detect
	// never reports "broken", only "found" or "not found" (see the
	// plan's design decision 9).
	Detect(home string) (Detection, bool, error)
	// ParseHook decodes payload (this host's own PreToolUse/
	// SessionStart JSON, read verbatim from stdin) into a HookEvent. A
	// non-nil error means payload is not valid JSON at all — the
	// Guardrails section's exit-2 case. A recognized JSON shape whose
	// event or tool name this adapter does not know about is not an
	// error: it decodes to EventUnknown, never fails.
	ParseHook(payload []byte) (HookEvent, error)
	// EncodeHook renders d in this host's own response shape.
	EncodeHook(d Decision) ([]byte, error)
	// HookConfig returns this host's hook configuration file, relative
	// to the plugin root (e.g. "hooks/claude-code.json").
	HookConfig() string
	// SkillsDirs returns this host's skill directories, in the order
	// the Hosts table names them.
	SkillsDirs() []string
}

// FromEnv picks the Adapter whose plugin-root environment variable is
// set in env (os.Environ()'s shape: "NAME=value" strings) — the one
// signal available to hook guard/hook session-start before they have
// read a single byte of stdin, since guard.sh/session-start.sh always
// run inside the calling host's own environment. Returns (nil, false)
// when neither or both are set — the phase plan's own "cannot be
// parsed at all" case (exit 2), since there is no shape to try
// parsing the payload as.
func FromEnv(env []string, claudeCode, cursor Adapter) (Adapter, bool) {
	_, claudeSet := lookupEnv(env, "CLAUDE_PLUGIN_ROOT")
	_, cursorSet := lookupEnv(env, "CURSOR_PLUGIN_ROOT")
	switch {
	case claudeSet && !cursorSet:
		return claudeCode, true
	case cursorSet && !claudeSet:
		return cursor, true
	default:
		return nil, false
	}
}

func lookupEnv(env []string, name string) (string, bool) {
	prefix := name + "="
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, prefix); ok {
			return v, true
		}
	}
	return "", false
}

// ErrUnparsable is returned by ParseHook (wrapped with context) when
// payload is not valid JSON — the trigger for hook guard/hook
// session-start's exit-2 path.
type ErrUnparsable struct {
	Host Host
	Err  error
}

func (e *ErrUnparsable) Error() string {
	return fmt.Sprintf("agenthost: %s: payload is not valid JSON: %v", e.Host, e.Err)
}

func (e *ErrUnparsable) Unwrap() error { return e.Err }
