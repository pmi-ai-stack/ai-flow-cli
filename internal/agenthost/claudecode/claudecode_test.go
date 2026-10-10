package claudecode_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/claudecode"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := agenthost.Fixture(agenthost.HostClaudeCode, name)
	if err != nil {
		t.Fatalf("Fixture(%s): %v", name, err)
	}
	return data
}

func TestParseHook_Bash(t *testing.T) {
	a := claudecode.New()
	ev, err := a.ParseHook(fixture(t, "bash.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Host != agenthost.HostClaudeCode {
		t.Errorf("Host = %v", ev.Host)
	}
	if ev.Event != agenthost.EventShell {
		t.Fatalf("Event = %v, want EventShell", ev.Event)
	}
	if ev.Command != "git push origin feature/PROJ-1" {
		t.Errorf("Command = %q", ev.Command)
	}
	if ev.Cwd != "/home/dev/acme.billing.api" {
		t.Errorf("Cwd = %q", ev.Cwd)
	}
}

func TestParseHook_Write(t *testing.T) {
	a := claudecode.New()
	ev, err := a.ParseHook(fixture(t, "write.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventWrite {
		t.Fatalf("Event = %v, want EventWrite", ev.Event)
	}
	if ev.FilePath != "/home/dev/.ai-flow/PROJ-1/plan.md" {
		t.Errorf("FilePath = %q", ev.FilePath)
	}
}

func TestParseHook_Edit(t *testing.T) {
	a := claudecode.New()
	ev, err := a.ParseHook(fixture(t, "edit.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventWrite {
		t.Fatalf("Event = %v, want EventWrite (Edit maps to write)", ev.Event)
	}
	if ev.FilePath != "/home/dev/.ai-flow/PROJ-1/status.json" {
		t.Errorf("FilePath = %q", ev.FilePath)
	}
}

func TestParseHook_MultiEdit(t *testing.T) {
	a := claudecode.New()
	ev, err := a.ParseHook(fixture(t, "multi-edit.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventWrite {
		t.Fatalf("Event = %v, want EventWrite (MultiEdit maps to write)", ev.Event)
	}
	if ev.FilePath != "/home/dev/.ai-flow/PROJ-1/status.json" {
		t.Errorf("FilePath = %q", ev.FilePath)
	}
}

func TestParseHook_NotebookEdit(t *testing.T) {
	a := claudecode.New()
	ev, err := a.ParseHook(fixture(t, "notebook-edit.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventWrite {
		t.Fatalf("Event = %v, want EventWrite (NotebookEdit maps to write)", ev.Event)
	}
	if ev.FilePath != "/home/dev/.ai-flow/cache/PROJ-1/analysis.ipynb" {
		t.Errorf("FilePath = %q, want notebook_path", ev.FilePath)
	}
}

func TestParseHook_SessionStart(t *testing.T) {
	a := claudecode.New()
	ev, err := a.ParseHook(fixture(t, "session-start.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventSessionStart {
		t.Fatalf("Event = %v, want EventSessionStart", ev.Event)
	}
	if ev.HostVersion != "2.1.0" {
		t.Errorf("HostVersion = %q", ev.HostVersion)
	}
}

func TestParseHook_Malformed(t *testing.T) {
	a := claudecode.New()
	_, err := a.ParseHook(fixture(t, "malformed.json"))
	if err == nil {
		t.Fatal("ParseHook(malformed): want error, got nil")
	}
	if _, ok := errors.AsType[*agenthost.ErrUnparsable](err); !ok {
		t.Fatalf("ParseHook(malformed): error is not *agenthost.ErrUnparsable: %v", err)
	}
}

func TestParseHook_UnrecognizedTool(t *testing.T) {
	a := claudecode.New()
	payload := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Glob","tool_input":{}}`)
	ev, err := a.ParseHook(payload)
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventUnknown {
		t.Fatalf("Event = %v, want EventUnknown", ev.Event)
	}
}

// TestEncodeHook_Allow pins the 2026-10-04 security fix: an allow is
// no output at all, which Claude Code documents as "no decision;
// normal permission flow applies". An explicit "allow" would skip the
// developer's own permission prompt for every Bash/Write/Edit call.
func TestEncodeHook_Allow(t *testing.T) {
	a := claudecode.New()
	data, err := a.EncodeHook(agenthost.Decision{Allow: true, Reason: "allow"})
	if err != nil {
		t.Fatalf("EncodeHook: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("EncodeHook(allow) = %s, want no output", data)
	}
}

func TestEncodeHook_Ask(t *testing.T) {
	a := claudecode.New()
	data, err := a.EncodeHook(agenthost.Decision{Ask: true, UserMessage: "approve?", AgentMessage: "approve?"})
	if err != nil {
		t.Fatalf("EncodeHook: %v", err)
	}
	var decoded struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	if decoded.HookSpecificOutput.PermissionDecision != "ask" {
		t.Errorf("permissionDecision = %q, want ask", decoded.HookSpecificOutput.PermissionDecision)
	}
	if decoded.HookSpecificOutput.PermissionDecisionReason != "approve?" {
		t.Errorf("permissionDecisionReason = %q", decoded.HookSpecificOutput.PermissionDecisionReason)
	}
}

func TestEncodeHook_Deny(t *testing.T) {
	a := claudecode.New()
	data, err := a.EncodeHook(agenthost.Decision{
		Allow: false, UserMessage: "blocked", AgentMessage: "blocked: use ai-flow git commit",
	})
	if err != nil {
		t.Fatalf("EncodeHook: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"permissionDecision":"deny"`) {
		t.Errorf("missing permissionDecision:deny in %s", s)
	}
	if !strings.Contains(s, `"user_message":"blocked"`) {
		t.Errorf("missing user_message in %s", s)
	}
	if !strings.Contains(s, `"agent_message":"blocked: use ai-flow git commit"`) {
		t.Errorf("missing agent_message in %s", s)
	}
}

func TestSkillsDirs(t *testing.T) {
	dirs := claudecode.New().SkillsDirs()
	if len(dirs) == 0 {
		t.Fatal("SkillsDirs: empty")
	}
	found := false
	for _, d := range dirs {
		if filepath.Base(d) == "skills" && filepath.Base(filepath.Dir(d)) == ".claude" {
			found = true
		}
	}
	if !found {
		t.Errorf("SkillsDirs %v: no .claude/skills entry", dirs)
	}
}

func TestDetect_NotFound(t *testing.T) {
	home := t.TempDir()
	det, found, err := claudecode.New().Detect(home)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if found {
		t.Fatalf("Detect: found = true, want false; det = %+v", det)
	}
}

func TestDetect_Found(t *testing.T) {
	home := t.TempDir()
	pluginDir := filepath.Join(home, "plugins", "ai-flow")
	manifestDir := filepath.Join(pluginDir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := `{"name":"ai-flow","version":"0.3.1"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	det, found, err := claudecode.New().Detect(home)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !found {
		t.Fatal("Detect: found = false, want true")
	}
	if det.Version != "0.3.1" {
		t.Errorf("Version = %q", det.Version)
	}
	if det.PluginRoot != pluginDir {
		t.Errorf("PluginRoot = %q, want %q", det.PluginRoot, pluginDir)
	}
}
