package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHooksInstall_ClaudeCodeProject_WritesSettings(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	stdout, _, exitCode := runRoot(t, "hooks", "install", "--host", "claude-code", "--scope", "project")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := settings["hooks"]; !ok {
		t.Fatalf("settings.json missing \"hooks\" key: %s", data)
	}
	var hooks claudeCodeHooksBlock
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatalf("decode hooks: %v", err)
	}
	if len(hooks.PreToolUse) != 2 || hooks.PreToolUse[0].Hooks[0].Command != "ai-flow hook guard" {
		t.Errorf("PreToolUse = %+v", hooks.PreToolUse)
	}
	// Every Claude Code tool that writes a file reaches the guard's
	// path rows — NotebookEdit and MultiEdit included (2026-10-04
	// security review #6). Same string as ai-flow/hooks/claude-code.json.
	if len(hooks.PreToolUse) == 2 && hooks.PreToolUse[1].Matcher != "Write|Edit|MultiEdit|NotebookEdit" {
		t.Errorf("write matcher = %q, want Write|Edit|MultiEdit|NotebookEdit", hooks.PreToolUse[1].Matcher)
	}
	if len(hooks.SessionStart) != 1 {
		t.Fatalf("SessionStart = %+v", hooks.SessionStart)
	}
	wantCmd := "ai-flow hook session-start --plugin-root " + dir
	if hooks.SessionStart[0].Hooks[0].Command != wantCmd {
		t.Errorf("SessionStart command = %q, want %q", hooks.SessionStart[0].Hooks[0].Command, wantCmd)
	}
}

func TestHooksInstall_ClaudeCodePreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	existing := `{"unrelatedSetting": true}`
	if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(existing), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, _, exitCode := runRoot(t, "hooks", "install", "--host", "claude-code", "--scope", "project"); exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(settings["unrelatedSetting"]) != "true" {
		t.Errorf("unrelatedSetting = %s, want preserved", settings["unrelatedSetting"])
	}
	if _, ok := settings["hooks"]; !ok {
		t.Error("hooks key missing after merge")
	}
}

func TestHooksInstall_CursorUser_WritesHooksFile(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	// os.UserHomeDir reads USERPROFILE on Windows, not HOME.
	t.Setenv("USERPROFILE", fakeHome)

	stdout, _, exitCode := runRoot(t, "hooks", "install", "--host", "cursor", "--scope", "user")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	path := filepath.Join(fakeHome, ".cursor", "hooks.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var file cursorHooksFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if file.Version != 1 {
		t.Errorf("Version = %d, want 1", file.Version)
	}
	if len(file.Hooks.PreToolUse) != 2 || !file.Hooks.PreToolUse[0].FailClosed {
		t.Errorf("PreToolUse = %+v", file.Hooks.PreToolUse)
	}
	// Write and Delete both reach the guard's path rows (phase 47);
	// same string as ai-flow/hooks/cursor.json.
	if len(file.Hooks.PreToolUse) == 2 && file.Hooks.PreToolUse[1].Matcher != "Write|Delete" {
		t.Errorf("write matcher = %q, want Write|Delete", file.Hooks.PreToolUse[1].Matcher)
	}
}

func TestHooksInstall_UnknownHost(t *testing.T) {
	_, _, exitCode := runRoot(t, "hooks", "install", "--host", "vscode", "--scope", "project")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)", exitCode)
	}
}

func TestHooksInstall_UnknownScope(t *testing.T) {
	_, _, exitCode := runRoot(t, "hooks", "install", "--host", "claude-code", "--scope", "global")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)", exitCode)
	}
}
