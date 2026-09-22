package agenthost_test

// TestNoSharedHookPayloadType proves the Guardrails section's own
// words — "There is no shared union object; testdata/hooks/<host>/
// fixtures prove each host parses its own shape" — at the wire-shape
// level: each host's ParseHook, given the *other* host's fixture, must
// never fully resolve it to the same Shell/Write event the owning host
// gets. Claude Code's shape (tool_name/tool_input) and Cursor's shape
// (tool/input) share no field names, so this also guards against a
// future refactor accidentally merging them into one payload struct.

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/claudecode"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/cursor"
)

func TestNoSharedHookPayloadType(t *testing.T) {
	claudeBash, err := agenthost.Fixture(agenthost.HostClaudeCode, "bash.json")
	if err != nil {
		t.Fatalf("Fixture: %v", err)
	}
	cursorShell, err := agenthost.Fixture(agenthost.HostCursor, "shell.json")
	if err != nil {
		t.Fatalf("Fixture: %v", err)
	}

	// Cursor's adapter, given Claude Code's Bash fixture: "hook_event_name"
	// matches ("PreToolUse" vs cursor's own "preToolUse" check fails), so
	// this must decode to EventUnknown, never EventShell.
	ev, err := cursor.New().ParseHook(claudeBash)
	if err != nil {
		t.Fatalf("cursor.ParseHook(claude-code bash.json): %v", err)
	}
	if ev.Event == agenthost.EventShell {
		t.Errorf("cursor.ParseHook(claude-code bash.json) resolved to EventShell — a shared union object exists")
	}

	// Claude Code's adapter, given Cursor's Shell fixture: same check, the
	// other direction.
	ev, err = claudecode.New().ParseHook(cursorShell)
	if err != nil {
		t.Fatalf("claudecode.ParseHook(cursor shell.json): %v", err)
	}
	if ev.Event == agenthost.EventShell {
		t.Errorf("claudecode.ParseHook(cursor shell.json) resolved to EventShell — a shared union object exists")
	}
}

func TestAdaptersImplementInterface(t *testing.T) {
	var _ agenthost.Adapter = claudecode.New()
	var _ agenthost.Adapter = cursor.New()
}
