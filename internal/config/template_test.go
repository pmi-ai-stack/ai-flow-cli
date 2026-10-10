package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// TestTemplateJiraIsTheEmbeddedTemplate pins `config init` without
// --tracker to exactly the bytes it wrote before phase 57.
func TestTemplateJiraIsTheEmbeddedTemplate(t *testing.T) {
	got, err := Template(TrackerJira)
	if err != nil {
		t.Fatalf("Template(jira): %v", err)
	}
	if string(got) != string(templateBytes) {
		t.Errorf("Template(jira) differs from template.json:\n%s", got)
	}
}

// TestTemplateGitHubPassesValidate: a fresh GitHub Issues config is
// valid out of the box, the same promise the Jira template keeps.
func TestTemplateGitHubPassesValidate(t *testing.T) {
	data, err := Template(TrackerGitHub)
	if err != nil {
		t.Fatalf("Template(github): %v", err)
	}
	cfg, err := decode(data)
	if err != nil {
		t.Fatalf("decode(Template(github)): %v\n%s", err, data)
	}
	if got := cfg.Tracker.TrackerProvider(); got != TrackerGitHub {
		t.Errorf("tracker provider = %q, want github", got)
	}
	if len(cfg.Tracker.Repositories) == 0 {
		t.Error("tracker.repositories is empty; config validate requires it for github")
	}
	if want := []string{"open"}; !reflect.DeepEqual(cfg.Tracker.Statuses.Todo, want) {
		t.Errorf("statuses.todo = %v, want %v (an open issue with no status label)", cfg.Tracker.Statuses.Todo, want)
	}
	warnings, err := Validate(cfg, "")
	if err != nil {
		t.Fatalf("Validate(github template) = %v, want nil", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Validate(github template) warnings = %v, want none", warnings)
	}
}

// TestTemplateGitHubOnlyReplacesTracker proves template.json stays the
// one full template: outside the tracker value, the GitHub template is
// template.json byte for byte.
func TestTemplateGitHubOnlyReplacesTracker(t *testing.T) {
	data, err := Template(TrackerGitHub)
	if err != nil {
		t.Fatalf("Template(github): %v", err)
	}
	var gh, jira map[string]json.RawMessage
	if err := json.Unmarshal(data, &gh); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(templateBytes, &jira); err != nil {
		t.Fatal(err)
	}
	if len(gh) != len(jira) {
		t.Fatalf("top-level keys: github %d, jira %d", len(gh), len(jira))
	}
	for k, v := range jira {
		if k == "tracker" {
			continue
		}
		if string(gh[k]) != string(v) {
			t.Errorf("%q differs from template.json", k)
		}
	}
	jStart, jEnd := trackerSpan(t, templateBytes)
	gStart, gEnd := trackerSpan(t, data)
	if string(data[:gStart]) != string(templateBytes[:jStart]) || string(data[gEnd:]) != string(templateBytes[jEnd:]) {
		t.Error("bytes outside the tracker value differ from template.json")
	}
	// The spliced value sits at template.json's nesting: every line after
	// its opening brace is indented under the "tracker" key.
	for line := range strings.SplitSeq(string(data[gStart:gEnd]), "\n") {
		if line != "{" && !strings.HasPrefix(line, "  ") {
			t.Errorf("tracker line %q is not indented to the template's nesting", line)
		}
	}
}

func trackerSpan(t *testing.T, data []byte) (int, int) {
	t.Helper()
	start, end, err := valueSpan(data, "tracker")
	if err != nil {
		t.Fatalf("valueSpan: %v", err)
	}
	return start, end
}

func TestTemplateUnknownTracker(t *testing.T) {
	if _, err := Template("gitlab"); err == nil {
		t.Fatal("Template(gitlab): want an error, got nil")
	}
}

func TestInitForGitHubIsByteStable(t *testing.T) {
	var got []string
	for range 2 {
		path, err := InitFor(t.TempDir(), "", TrackerGitHub)
		if err != nil {
			t.Fatalf("InitFor(github): %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(data))
	}
	if got[0] != got[1] {
		t.Error("InitFor(github) produced different bytes across two runs")
	}
	if !strings.Contains(got[0], `"provider": "github"`) {
		t.Errorf("written config has no github provider:\n%s", got[0])
	}
}
