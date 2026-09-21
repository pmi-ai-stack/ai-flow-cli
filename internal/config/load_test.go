package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func writeConfigFile(t *testing.T, dir string, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal fixture: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func envelopeCode(t *testing.T, err error) string {
	t.Helper()
	var ee *envelope.Error
	if !errors.As(err, &ee) {
		t.Fatalf("error type = %T, want *envelope.Error: %v", err, err)
	}
	return ee.Code
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, "")
	if err == nil {
		t.Fatal("Load: want an error for a missing config.json, got nil")
	}
	if code := envelopeCode(t, err); code != "missing_artifact" {
		t.Errorf("error.code = %q, want %q", code, "missing_artifact")
	}
}

func TestLoadUnknownField(t *testing.T) {
	dir := t.TempDir()
	raw := `{"schema_version": 1, "not_a_real_field": true}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := Load(dir, "")
	if err == nil {
		t.Fatal("Load: want an error for an unknown field, got nil")
	}
	if code := envelopeCode(t, err); code != "config_invalid" {
		t.Errorf("error.code = %q, want %q", code, "config_invalid")
	}
}

func TestLoadWrongSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, map[string]any{"schema_version": 2})
	_, err := Load(dir, "")
	if err == nil {
		t.Fatal("Load: want an error for schema_version 2, got nil")
	}
	if code := envelopeCode(t, err); code != "config_invalid" {
		t.Errorf("error.code = %q, want %q", code, "config_invalid")
	}
}

func TestLoadMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := Load(dir, "")
	if code := envelopeCode(t, err); code != "config_invalid" {
		t.Errorf("error.code = %q, want %q", code, "config_invalid")
	}
}

func TestLoadAppliesGuardIdleHoursDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig()
	cfg.Defaults.GuardIdleHours = 0
	writeConfigFile(t, dir, cfg)

	got, err := Load(dir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Defaults.GuardIdleHours != defaultGuardIdleHours {
		t.Errorf("GuardIdleHours = %d, want backfilled default %d", got.Defaults.GuardIdleHours, defaultGuardIdleHours)
	}
}

func TestLoadPreservesExplicitGuardIdleHours(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig()
	cfg.Defaults.GuardIdleHours = 24
	writeConfigFile(t, dir, cfg)

	got, err := Load(dir, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Defaults.GuardIdleHours != 24 {
		t.Errorf("GuardIdleHours = %d, want explicit 24 preserved", got.Defaults.GuardIdleHours)
	}
}

func TestResolvePath(t *testing.T) {
	if got, want := ResolvePath("/home/x/.ai-flow", ""), filepath.Join("/home/x/.ai-flow", "config.json"); got != want {
		t.Errorf("ResolvePath(home, \"\") = %q, want %q", got, want)
	}
	if got, want := ResolvePath("/home/x/.ai-flow", "/elsewhere/config.json"), "/elsewhere/config.json"; got != want {
		t.Errorf("ResolvePath(home, override) = %q, want %q", got, want)
	}
}

func TestLoadUsesConfigFlagOverride(t *testing.T) {
	dir := t.TempDir()
	explicitPath := filepath.Join(dir, "custom.json")
	data, err := json.Marshal(validConfig())
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(explicitPath, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// homeDir points somewhere with no config.json at all — Load must
	// use the --config override, not <home>/config.json.
	got, err := Load(t.TempDir(), explicitPath)
	if err != nil {
		t.Fatalf("Load with --config override: %v", err)
	}
	if got.Tracker.Provider != "jira" {
		t.Errorf("loaded config does not match the fixture written at the override path")
	}
}
