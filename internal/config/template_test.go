package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestTemplatePassesValidate is the phase's stated Done when: `config
// init && config validate` passes on the embedded template.
func TestTemplatePassesValidate(t *testing.T) {
	cfg, err := decode(templateBytes)
	if err != nil {
		t.Fatalf("decode(templateBytes): %v", err)
	}
	warnings, err := Validate(cfg, "")
	if err != nil {
		t.Fatalf("Validate(template) = %v, want nil — the template must be valid out of the box", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Validate(template) warnings = %v, want none", warnings)
	}
}

// TestTemplateByteStable proves Init's write is deterministic: two
// fresh homes both get the exact same bytes.
func TestTemplateByteStable(t *testing.T) {
	pathA, err := Init(t.TempDir(), "")
	if err != nil {
		t.Fatalf("Init (a): %v", err)
	}
	pathB, err := Init(t.TempDir(), "")
	if err != nil {
		t.Fatalf("Init (b): %v", err)
	}

	dataA, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("read %s: %v", pathA, err)
	}
	dataB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatalf("read %s: %v", pathB, err)
	}
	if string(dataA) != string(dataB) {
		t.Errorf("Init produced different bytes across two runs:\n--- a ---\n%s\n--- b ---\n%s", dataA, dataB)
	}
}

func TestInitWritesReadableConfig(t *testing.T) {
	home := t.TempDir()
	path, err := Init(home, "")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if want := filepath.Join(home, "config.json"); path != want {
		t.Errorf("Init path = %q, want %q", path, want)
	}

	cfg, err := Load(home, "")
	if err != nil {
		t.Fatalf("Load after Init: %v", err)
	}
	if cfg.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("loaded schema_version = %d, want %d", cfg.SchemaVersion, CurrentSchemaVersion)
	}
}

func TestConfigInitRefusesExisting(t *testing.T) {
	home := t.TempDir()
	if _, err := Init(home, ""); err != nil {
		t.Fatalf("first Init: %v", err)
	}

	_, err := Init(home, "")
	if err == nil {
		t.Fatal("second Init: want a refusal, got nil")
	}
	if code := envelopeCode(t, err); code != "usage" {
		t.Errorf("error.code = %q, want %q", code, "usage")
	}
}

func TestInitUsesConfigFlagOverride(t *testing.T) {
	dir := t.TempDir()
	explicitPath := filepath.Join(dir, "nested", "custom.json")

	path, err := Init(t.TempDir(), explicitPath)
	if err != nil {
		t.Fatalf("Init with --config override: %v", err)
	}
	if path != explicitPath {
		t.Errorf("Init path = %q, want %q", path, explicitPath)
	}
	if _, err := os.Stat(explicitPath); err != nil {
		t.Errorf("expected file at %s: %v", explicitPath, err)
	}
}

func TestFromTemplateMatchesInitThenLoad(t *testing.T) {
	home := t.TempDir()
	if _, err := Init(home, ""); err != nil {
		t.Fatalf("Init: %v", err)
	}
	loaded, err := Load(home, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := FromTemplate(); !reflect.DeepEqual(got, loaded) {
		t.Errorf("FromTemplate() = %+v, want what Init+Load gives: %+v", got, loaded)
	}
	a, b := FromTemplate(), FromTemplate()
	a.Ops.RiskGate = -1
	if b.Ops.RiskGate == -1 {
		t.Error("FromTemplate returns shared state; a caller's edit leaked into the next call")
	}
}
