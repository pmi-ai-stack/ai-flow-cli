package standards

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func TestLoadValid(t *testing.T) {
	m, err := Load("testdata/packs/valid")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.ID != "fixture" {
		t.Errorf("ID = %q, want %q", m.ID, "fixture")
	}
	if len(m.Sections) != 1 {
		t.Errorf("len(Sections) = %d, want 1", len(m.Sections))
	}
	if len(m.Checks) != 1 {
		t.Errorf("len(Checks) = %d, want 1", len(m.Checks))
	}
}

func TestLoadMissingManifest(t *testing.T) {
	_, err := Load("testdata/packs/no-manifest")
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("Load(no manifest) error type = %T, want *envelope.Error", err)
	}
	if envErr.Code != "pack_invalid" {
		t.Errorf("Code = %q, want %q", envErr.Code, "pack_invalid")
	}
}

func TestLoadBadShape(t *testing.T) {
	_, err := Load("testdata/packs/invalid-bad-shape")
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("Load(bad shape) error type = %T, want *envelope.Error", err)
	}
	if envErr.Code != "pack_invalid" {
		t.Errorf("Code = %q, want %q", envErr.Code, "pack_invalid")
	}
	if len(envErr.Details) == 0 {
		t.Error("Details is empty, want the schema failure(s)")
	}
}

// TestManifestCarriesOverrides proves the phase-13 addition to the
// pack format loads: a repository's own manifest declaring, with a
// reason, which less-specific `must` entries it replaces.
func TestManifestCarriesOverrides(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "guidance", "errors.md"), "# Errors\n")
	writeFile(t, filepath.Join(dir, ManifestFile), `{
  "schema_version": 1, "id": "acme.billing.api", "version": "0.1.0",
  "sections": [
    { "id": "go.errors", "path": "guidance/errors.md", "severity": "must" }
  ],
  "checks": [],
  "overrides": [
    { "id": "go.errors", "reason": "this service carries trace context on every error" }
  ]
}`)

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Overrides) != 1 {
		t.Fatalf("Overrides = %+v, want exactly one", m.Overrides)
	}
	if m.Overrides[0].ID != "go.errors" || m.Overrides[0].Reason == "" {
		t.Errorf("Overrides[0] = %+v, want go.errors with a non-empty reason", m.Overrides[0])
	}
}

// TestOverrideWithoutAReasonIsRejected proves the reason is not
// optional: an overrides[] entry *is* the approval record for
// replacing an org must (the phase-13 plan's design decision 14), so
// an entry with no reason records nothing.
func TestOverrideWithoutAReasonIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ManifestFile), `{
  "schema_version": 1, "id": "x", "version": "1.0.0",
  "sections": [], "checks": [],
  "overrides": [{ "id": "go.errors" }]
}`)

	_, err := Load(dir)
	var envErr *envelope.Error
	if err == nil || !errors.As(err, &envErr) || envErr.Code != "pack_invalid" {
		t.Fatalf("Load = %v, want pack_invalid", err)
	}
}

// TestBuiltInCheckNeedsNoTool proves the phase-13 schema correction:
// the spec's own manifest example has a `commit` check with no tool at
// all, and check_runner_missing has no binary to look for.
func TestBuiltInCheckNeedsNoTool(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ManifestFile), `{
  "schema_version": 1, "id": "x", "version": "1.0.0",
  "sections": [],
  "checks": [{ "id": "commit.conventional", "kind": "commit", "severity": "must", "applies_to": { "stages": ["ship"] } }]
}`)

	if _, err := Validate(dir, []string{}); err != nil {
		t.Fatalf("Validate on a tool-less built-in check: %v", err)
	}
}
