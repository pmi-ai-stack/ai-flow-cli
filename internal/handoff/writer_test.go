package handoff

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// requireByteStable duplicates handofftest.RequireByteStable's logic
// for this package's own tests — see active_test.go's fixedClock for
// why: handofftest imports internal/handoff, so an internal
// (same-package) test file here cannot import it back without an
// import cycle.
func requireByteStable(t *testing.T, write func() ([]byte, error)) []byte {
	t.Helper()
	first, err := write()
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	second, err := write()
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("not byte-stable:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	return first
}

func TestWriteArtifactRejectsSchemaViolation(t *testing.T) {
	dir := t.TempDir()
	bad := &Status{Key: "not-a-valid-key", Stages: NewStatus("X-1").Stages}
	_, err := WriteArtifact(dir, "status.json", "status", bad)
	if err == nil {
		t.Fatal("want a schema validation error, got nil")
	}
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "invalid_result" {
		t.Errorf("err = %v, want *envelope.Error{Code: invalid_result}", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "status.json")); statErr == nil {
		t.Error("status.json should not have been written after a schema failure")
	}
}

func TestWriteArtifactAndReadArtifactRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStatus("PROJ-1")
	path, err := WriteArtifact(dir, "status.json", "status", s)
	if err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	if path != filepath.Join(dir, "status.json") {
		t.Errorf("path = %q, want %q", path, filepath.Join(dir, "status.json"))
	}
	data, err := ReadArtifact(path)
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ReadArtifact returned no data")
	}
}

func TestWriteRawRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := &Active{Key: "PROJ-1", Stage: "fetch"}
	path, err := WriteRaw(dir, "active.json", a)
	if err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	data, err := ReadArtifact(path)
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ReadArtifact returned no data")
	}
}

func TestReadArtifactMissingFile(t *testing.T) {
	dir := t.TempDir()
	data, err := ReadArtifact(filepath.Join(dir, "does-not-exist.json"))
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if data != nil {
		t.Errorf("data = %v, want nil for a missing file", data)
	}
}

func TestMarshalCanonicalIsByteStable(t *testing.T) {
	s := NewStatus("PROJ-1")
	s.Entry(StagePlan).State = StateApproved
	s.Entry(StagePlan).Approval = &Approval{By: "alice", At: "2026-01-01T00:00:00Z", Gate: GatePost}

	data := requireByteStable(t, func() ([]byte, error) {
		return marshalCanonical(s)
	})

	if data[len(data)-1] != '\n' {
		t.Error("marshalCanonical output must end with a trailing newline")
	}
}

func TestMarshalCanonicalActiveIsByteStable(t *testing.T) {
	a := &Active{Key: "PROJ-1", Stage: "plan", RepoRoots: []string{}, ToolConfigFiles: []string{}, AllowlistHash: "abc", LastEventAt: "2026-01-01T00:00:00Z"}
	requireByteStable(t, func() ([]byte, error) {
		return marshalCanonical(a)
	})
}
