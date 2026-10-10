package cursor_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/cursor"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := agenthost.Fixture(agenthost.HostCursor, name)
	if err != nil {
		t.Fatalf("Fixture(%s): %v", name, err)
	}
	return data
}

// The fixtures follow Cursor's documented hook input
// (cursor.com/docs/agent/hooks): tool_name/tool_input on preToolUse,
// cursor_version among the common fields. Phase 9's hand-built
// tool/input guess never matched it, so every real event decoded as
// unknown and the guard allowed it (phase 47).

func TestParseHook_Shell(t *testing.T) {
	a := cursor.New()
	ev, err := a.ParseHook(fixture(t, "shell.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Host != agenthost.HostCursor {
		t.Errorf("Host = %v", ev.Host)
	}
	if ev.Event != agenthost.EventShell {
		t.Fatalf("Event = %v, want EventShell", ev.Event)
	}
	if ev.Command != "git push origin feature/PROJ-1" {
		t.Errorf("Command = %q", ev.Command)
	}
	// tool_input.working_directory is where the command runs; it wins
	// over the top-level cwd.
	if ev.Cwd != "/home/dev/acme.billing.api" {
		t.Errorf("Cwd = %q, want tool_input.working_directory", ev.Cwd)
	}
}

func TestParseHook_ShellCwdFallbacks(t *testing.T) {
	a := cursor.New()
	cases := map[string]struct{ payload, want string }{
		"cwd when no working_directory": {
			`{"hook_event_name":"preToolUse","tool_name":"Shell","tool_input":{"command":"ls"},"cwd":"/repo"}`, "/repo"},
		"first workspace root when no cwd": {
			`{"hook_event_name":"preToolUse","tool_name":"Shell","tool_input":{"command":"ls"},"workspace_roots":["/ws","/other"]}`, "/ws"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ev, err := a.ParseHook([]byte(tc.payload))
			if err != nil {
				t.Fatalf("ParseHook: %v", err)
			}
			if ev.Cwd != tc.want {
				t.Errorf("Cwd = %q, want %q", ev.Cwd, tc.want)
			}
		})
	}
}

func TestParseHook_Write(t *testing.T) {
	a := cursor.New()
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
	if ev.Cwd != "/home/dev/acme.billing.api" {
		t.Errorf("Cwd = %q", ev.Cwd)
	}
}

// TestParseHook_Delete: deleting a protected file is a write to it.
func TestParseHook_Delete(t *testing.T) {
	a := cursor.New()
	ev, err := a.ParseHook(fixture(t, "delete.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventWrite {
		t.Fatalf("Event = %v, want EventWrite (Delete maps to write)", ev.Event)
	}
	if ev.FilePath != "/home/dev/.ai-flow/PROJ-1/status.json" {
		t.Errorf("FilePath = %q", ev.FilePath)
	}
}

// TestParseHook_WritePathFields: Cursor does not document the path
// field inside tool_input; file_path, then path, then target_file are
// read, and anything else leaves FilePath empty — which the guard
// allows and logs as hook_shape_unknown instead of misreading it.
func TestParseHook_WritePathFields(t *testing.T) {
	a := cursor.New()
	cases := map[string]string{
		`{"file_path":"/a","path":"/b","target_file":"/c"}`: "/a",
		`{"path":"/b","target_file":"/c"}`:                  "/b",
		`{"target_file":"/c"}`:                              "/c",
		`{"filename":"/d"}`:                                 "",
	}
	for input, want := range cases {
		ev, err := a.ParseHook([]byte(`{"hook_event_name":"preToolUse","tool_name":"Write","tool_input":` + input + `}`))
		if err != nil {
			t.Fatalf("ParseHook(%s): %v", input, err)
		}
		if ev.Event != agenthost.EventWrite || ev.FilePath != want {
			t.Errorf("tool_input %s: Event = %v, FilePath = %q, want write %q", input, ev.Event, ev.FilePath, want)
		}
	}
}

// TestParseHook_Phase9GuessIsNotDecoded pins that the never-observed
// tool/input shape is not silently supported.
func TestParseHook_Phase9GuessIsNotDecoded(t *testing.T) {
	a := cursor.New()
	ev, err := a.ParseHook([]byte(`{"hook_event_name":"preToolUse","cwd":"/repo","tool":"Shell","input":{"command":"git push"}}`))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventUnknown {
		t.Errorf("Event = %v, want EventUnknown", ev.Event)
	}
}

func TestParseHook_SessionStart(t *testing.T) {
	a := cursor.New()
	ev, err := a.ParseHook(fixture(t, "session-start.json"))
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventSessionStart {
		t.Fatalf("Event = %v, want EventSessionStart", ev.Event)
	}
	if ev.HostVersion != "2.1.0" {
		t.Errorf("HostVersion = %q, want cursor_version", ev.HostVersion)
	}
}

func TestParseHook_Malformed(t *testing.T) {
	a := cursor.New()
	_, err := a.ParseHook(fixture(t, "malformed.json"))
	if err == nil {
		t.Fatal("ParseHook(malformed): want error, got nil")
	}
	if _, ok := errors.AsType[*agenthost.ErrUnparsable](err); !ok {
		t.Fatalf("ParseHook(malformed): error is not *agenthost.ErrUnparsable: %v", err)
	}
}

func TestParseHook_UnrecognizedTool(t *testing.T) {
	a := cursor.New()
	payload := []byte(`{"hook_event_name":"preToolUse","tool_name":"Grep","tool_input":{}}`)
	ev, err := a.ParseHook(payload)
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if ev.Event != agenthost.EventUnknown {
		t.Fatalf("Event = %v, want EventUnknown", ev.Event)
	}
}

func TestEncodeHook_Allow(t *testing.T) {
	a := cursor.New()
	data, err := a.EncodeHook(agenthost.Decision{Allow: true})
	if err != nil {
		t.Fatalf("EncodeHook: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded["permission"] != "allow" {
		t.Errorf("permission = %v, want allow", decoded["permission"])
	}
}

// TestEncodeHook_AskIsDeny: Cursor accepts "ask" on preToolUse but
// does not enforce it, so an ask (gate approval) is encoded as a deny
// whose messages carry the guidance — never as an allow.
func TestEncodeHook_AskIsDeny(t *testing.T) {
	a := cursor.New()
	data, err := a.EncodeHook(agenthost.Decision{Ask: true, UserMessage: "run it yourself", AgentMessage: "ask the developer"})
	if err != nil {
		t.Fatalf("EncodeHook: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"permission":"deny"`) {
		t.Errorf("ask must encode as deny on Cursor, got %s", s)
	}
	if !strings.Contains(s, `"agent_message":"ask the developer"`) {
		t.Errorf("missing agent_message in %s", s)
	}
}

func TestEncodeHook_Deny(t *testing.T) {
	a := cursor.New()
	data, err := a.EncodeHook(agenthost.Decision{
		Allow: false, UserMessage: "blocked", AgentMessage: "blocked: use ai-flow git commit",
	})
	if err != nil {
		t.Fatalf("EncodeHook: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"permission":"deny"`) {
		t.Errorf("missing permission:deny in %s", s)
	}
	if !strings.Contains(s, `"user_message":"blocked"`) {
		t.Errorf("missing user_message in %s", s)
	}
}

func TestSkillsDirs(t *testing.T) {
	dirs := cursor.New().SkillsDirs()
	found := false
	for _, d := range dirs {
		if filepath.Base(d) == "skills" && filepath.Base(filepath.Dir(d)) == ".claude" {
			found = true
		}
	}
	if !found {
		t.Errorf("SkillsDirs %v: no .claude/skills compatibility entry", dirs)
	}
}

func TestDetect_NotFound(t *testing.T) {
	home := t.TempDir()
	det, found, err := cursor.New().Detect(home)
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
	manifestDir := filepath.Join(pluginDir, ".cursor-plugin")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := `{"name":"ai-flow","version":"0.3.1"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	det, found, err := cursor.New().Detect(home)
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
