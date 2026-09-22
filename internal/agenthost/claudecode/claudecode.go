// Package claudecode is the Claude Code agenthost.Adapter: its own
// PreToolUse/SessionStart payload shape in, its own hookSpecificOutput
// response shape out. Nothing here is shared with
// internal/agenthost/cursor — see the plan's design decision 1.
package claudecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
)

// Adapter implements agenthost.Adapter for Claude Code.
type Adapter struct{}

var _ agenthost.Adapter = Adapter{}

// New returns a Claude Code Adapter.
func New() Adapter { return Adapter{} }

// Host returns agenthost.HostClaudeCode.
func (Adapter) Host() agenthost.Host { return agenthost.HostClaudeCode }

// HookConfig returns Claude Code's hook configuration file, relative
// to the plugin root.
func (Adapter) HookConfig() string { return "hooks/claude-code.json" }

// SkillsDirs returns Claude Code's skill directories, in the order the
// Hosts table names them: user, project, plugin.
func (Adapter) SkillsDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(".claude", "skills"),
		// "<plugin>/skills/" is resolved by the caller, which knows the
		// plugin root (from Detect or from --plugin-root); this adapter
		// has no plugin root of its own to resolve it against.
	}
}

// payload is Claude Code's own PreToolUse/SessionStart hook input
// shape — this adapter's own wire type, never exported, never shared
// with internal/agenthost/cursor's equivalent.
type payload struct {
	HookEventName string `json:"hook_event_name"`
	Cwd           string `json:"cwd"`
	ToolName      string `json:"tool_name"`
	ToolInput     struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
	// Version is this adapter's provisional reading of SessionStart's
	// own-host-version field — unconfirmed against the real product,
	// see the plan's design decision 8.
	Version string `json:"version"`
}

// ParseHook decodes a Claude Code PreToolUse or SessionStart payload.
// A non-nil error means payload is not valid JSON at all (exit 2 for
// hook guard/hook session-start); an unrecognized hook_event_name or
// tool_name decodes to agenthost.EventUnknown, never an error.
func (Adapter) ParseHook(raw []byte) (agenthost.HookEvent, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var p payload
	if err := dec.Decode(&p); err != nil {
		return agenthost.HookEvent{}, &agenthost.ErrUnparsable{Host: agenthost.HostClaudeCode, Err: err}
	}

	ev := agenthost.HookEvent{Host: agenthost.HostClaudeCode, Cwd: p.Cwd}

	switch p.HookEventName {
	case "SessionStart":
		ev.Event = agenthost.EventSessionStart
		ev.HostVersion = p.Version
		return ev, nil
	case "PreToolUse":
		switch p.ToolName {
		case "Bash":
			ev.Event = agenthost.EventShell
			ev.Command = p.ToolInput.Command
		case "Write", "Edit":
			ev.Event = agenthost.EventWrite
			ev.FilePath = p.ToolInput.FilePath
		default:
			ev.Event = agenthost.EventUnknown
		}
		return ev, nil
	default:
		ev.Event = agenthost.EventUnknown
		return ev, nil
	}
}

// hookSpecificOutput is Claude Code's own PreToolUse response shape.
type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

type response struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
	UserMessage        string             `json:"user_message"`
	AgentMessage       string             `json:"agent_message"`
}

// EncodeHook renders d as Claude Code's own
// hookSpecificOutput.permissionDecision shape.
func (Adapter) EncodeHook(d agenthost.Decision) ([]byte, error) {
	decision := "deny"
	if d.Allow {
		decision = "allow"
	}
	resp := response{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: d.AgentMessage,
		},
		UserMessage:  d.UserMessage,
		AgentMessage: d.AgentMessage,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("claudecode: marshal hook response: %w", err)
	}
	return data, nil
}

// Detect looks for .claude-plugin/plugin.json under home's Claude Code
// skill/config locations (~/.claude/plugins/ai-flow being the one this
// repository's own install path uses) and, when a plugin repository
// checkout sits alongside the CLI's own working directory (the
// monorepo layout this workspace itself uses), under "../ai-flow" too
// — see the plan's design decision 9: "not found" is not an error.
// The candidate-scan plumbing itself is shared with cursor.Adapter's
// own Detect via agenthost.ScanForManifest; only the candidate list
// and the manifest's own relative path differ per host.
func (Adapter) Detect(home string) (agenthost.Detection, bool, error) {
	candidates := []string{
		filepath.Join(home, "plugins", "ai-flow"),
		filepath.Join(filepath.Dir(home), "ai-flow"),
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(userHome, ".claude", "plugins", "ai-flow"))
	}

	det, found, err := agenthost.ScanForManifest(candidates, filepath.Join(".claude-plugin", "plugin.json"))
	if err != nil {
		return agenthost.Detection{}, false, fmt.Errorf("claudecode: %w", err)
	}
	return det, found, nil
}
