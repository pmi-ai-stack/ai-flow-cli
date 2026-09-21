package handoff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
)

// marshalCanonical is 07-determinism.md's write shape for every
// machine-readable artifact: two-space indent, HTML escaping disabled
// (the same reason internal/envelope disables it — content may
// legitimately contain "<" or ">"), a trailing newline, and — because
// encoding/json already sorts map[string]T keys and preserves declared
// struct field order — no further canonicalization is needed for
// "sorted keys" to hold.
func marshalCanonical(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("handoff: marshal: %w", err)
	}
	return buf.Bytes(), nil // json.Encoder.Encode already appends exactly one trailing newline
}

// writeJSON writes data to dir/filename as a temp file in the same
// directory, synced and closed, then renamed into place — so a crash
// mid-write never leaves a partial artifact. Mirrors
// internal/config/template.go's atomicWrite.
func writeJSON(dir, filename string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("handoff: create %s: %w", dir, err)
	}

	path := dir + string(os.PathSeparator) + filename
	tmp, err := os.CreateTemp(dir, "."+filename+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("handoff: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("handoff: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("handoff: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("handoff: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("handoff: rename into place: %w", err)
	}
	cleanup = false
	return path, nil
}

// WriteArtifact marshals v as canonical JSON, validates it against
// artifact's embedded schema, then writes it atomically to
// dir/filename. artifact must be one of schema.Artifacts() — every
// caller in this package passes a compile-time-known constant, so a
// mismatch here is this package's own bug, reported as "invalid_result"
// rather than a caller-facing validation failure.
func WriteArtifact(dir, filename, artifact string, v any) (string, error) {
	data, err := marshalCanonical(v)
	if err != nil {
		return "", err
	}
	if err := schema.Validate(artifact, data); err != nil {
		return "", &envelope.Error{
			Code:    "invalid_result",
			Message: fmt.Sprintf("handoff: %s does not satisfy its own %s schema: %v", filename, artifact, err),
		}
	}
	return writeJSON(dir, filename, data)
}

// WriteRaw marshals v as canonical JSON and writes it atomically to
// dir/filename, with no schema step — for active.json, which is not
// one of the 17 handoff artifacts (it lives outside ticket
// directories and has no embedded schema).
func WriteRaw(dir, filename string, v any) (string, error) {
	data, err := marshalCanonical(v)
	if err != nil {
		return "", err
	}
	return writeJSON(dir, filename, data)
}

// ReadArtifact reads path, returning (nil, nil) when it does not
// exist — every loader in this package treats a missing artifact as
// "not written yet", not as an error, so callers branch on a nil
// slice rather than on errors.Is(err, os.ErrNotExist) themselves.
func ReadArtifact(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is always built from --home/a ticket key, explicit CLI input, not attacker-controlled
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("handoff: read %s: %w", path, err)
	}
	return data, nil
}
