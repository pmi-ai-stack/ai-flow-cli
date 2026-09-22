package standards

import (
	"errors"
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
