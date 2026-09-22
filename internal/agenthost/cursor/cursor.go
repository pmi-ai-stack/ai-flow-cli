// Package cursor is the Cursor agenthost.Adapter: its own
// preToolUse/sessionStart payload shape in, its own permission
// response shape out. Nothing here is shared with
// internal/agenthost/claudecode — see the plan's design decision 1.
//
// Cursor's real preToolUse field names (in particular for the Write
// matcher) are unconfirmed against the product — see the plan's
// design decision 5. What this package parses is a provisional,
// internally-consistent, fully-tested shape, flagged in the
// phase-9 post-report as pending a real recorded payload.
package cursor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
)

// Adapter implements agenthost.Adapter for Cursor.
type Adapter struct{}

var _ agenthost.Adapter = Adapter{}

// New returns a Cursor Adapter.
func New() Adapter { return Adapter{} }

// Host returns agenthost.HostCursor.
func (Adapter) Host() agenthost.Host { return agenthost.HostCursor }

// HookConfig returns Cursor's hook configuration file, relative to the
// plugin root.
func (Adapter) HookConfig() string { return "hooks/cursor.json" }

// SkillsDirs returns Cursor's skill directories, in the order the
// Hosts table names them: user, project (.agents/skills and
// .cursor/skills, each's ~ form), plus .claude/skills for
// compatibility.
func (Adapter) SkillsDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(".agents", "skills"),
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(".cursor", "skills"),
		filepath.Join(".claude", "skills"),
	}
}

// payload is Cursor's own preToolUse/sessionStart hook input shape —
// this adapter's own wire type, never exported, never shared with
// internal/agenthost/claudecode's equivalent.
type payload struct {
	HookEventName string `json:"hook_event_name"`
	Cwd           string `json:"cwd"`
	Tool          string `json:"tool"`
	Input         struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
	} `json:"input"`
	PluginRoot string `json:"plugin_root"`
	// AppVersion is this adapter's provisional reading of
	// sessionStart's own-host-version field — unconfirmed against the
	// real product, see the plan's design decision 8.
	AppVersion string `json:"app_version"`
}

// ParseHook decodes a Cursor preToolUse or sessionStart payload. A
// non-nil error means payload is not valid JSON at all (exit 2 for
// hook guard/hook session-start); an unrecognized hook_event_name or
// tool decodes to agenthost.EventUnknown, never an error.
func (Adapter) ParseHook(raw []byte) (agenthost.HookEvent, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var p payload
	if err := dec.Decode(&p); err != nil {
		return agenthost.HookEvent{}, &agenthost.ErrUnparsable{Host: agenthost.HostCursor, Err: err}
	}

	ev := agenthost.HookEvent{Host: agenthost.HostCursor, Cwd: p.Cwd}

	switch p.HookEventName {
	case "sessionStart":
		ev.Event = agenthost.EventSessionStart
		ev.PluginRoot = p.PluginRoot
		ev.HostVersion = p.AppVersion
		return ev, nil
	case "preToolUse":
		switch p.Tool {
		case "Shell":
			ev.Event = agenthost.EventShell
			ev.Command = p.Input.Command
		case "Write":
			ev.Event = agenthost.EventWrite
			ev.FilePath = p.Input.FilePath
		default:
			ev.Event = agenthost.EventUnknown
		}
		return ev, nil
	default:
		ev.Event = agenthost.EventUnknown
		return ev, nil
	}
}

// response is Cursor's own preToolUse response shape.
type response struct {
	Permission   string `json:"permission"`
	UserMessage  string `json:"user_message"`
	AgentMessage string `json:"agent_message"`
}

// EncodeHook renders d as Cursor's own permission shape.
func (Adapter) EncodeHook(d agenthost.Decision) ([]byte, error) {
	permission := "deny"
	if d.Allow {
		permission = "allow"
	}
	resp := response{
		Permission:   permission,
		UserMessage:  d.UserMessage,
		AgentMessage: d.AgentMessage,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("cursor: marshal hook response: %w", err)
	}
	return data, nil
}

// Detect looks for .cursor-plugin/plugin.json under home's Cursor
// plugin locations — see claudecode.Adapter.Detect's own doc comment;
// the same candidate-list, no-error-on-absence shape (both share
// agenthost.ScanForManifest's plumbing), mirrored for Cursor's own
// local-plugin install path (~/.cursor/plugins/local/ai-flow).
func (Adapter) Detect(home string) (agenthost.Detection, bool, error) {
	candidates := []string{
		filepath.Join(home, "plugins", "ai-flow"),
		filepath.Join(filepath.Dir(home), "ai-flow"),
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(userHome, ".cursor", "plugins", "local", "ai-flow"))
	}

	det, found, err := agenthost.ScanForManifest(candidates, filepath.Join(".cursor-plugin", "plugin.json"))
	if err != nil {
		return agenthost.Detection{}, false, fmt.Errorf("cursor: %w", err)
	}
	return det, found, nil
}
