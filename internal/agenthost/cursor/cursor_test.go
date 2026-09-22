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
	if ev.Cwd != "/home/dev/acme.billing.api" {
		t.Errorf("Cwd = %q", ev.Cwd)
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
	if ev.PluginRoot != "/home/dev/.cursor/plugins/local/ai-flow" {
		t.Errorf("PluginRoot = %q", ev.PluginRoot)
	}
	if ev.HostVersion != "1.5.0" {
		t.Errorf("HostVersion = %q", ev.HostVersion)
	}
}

func TestParseHook_Malformed(t *testing.T) {
	a := cursor.New()
	_, err := a.ParseHook(fixture(t, "malformed.json"))
	if err == nil {
		t.Fatal("ParseHook(malformed): want error, got nil")
	}
	var unparsable *agenthost.ErrUnparsable
	if !errors.As(err, &unparsable) {
		t.Fatalf("ParseHook(malformed): error is not *agenthost.ErrUnparsable: %v", err)
	}
}

func TestParseHook_UnrecognizedTool(t *testing.T) {
	a := cursor.New()
	payload := []byte(`{"hook_event_name":"preToolUse","tool":"Glob","input":{}}`)
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
