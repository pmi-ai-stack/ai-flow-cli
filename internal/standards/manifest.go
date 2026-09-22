package standards

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
)

// ManifestFile is the manifest's fixed filename inside a pack
// directory — a pack has no other configurable entry point.
const ManifestFile = "standards.json"

// AppliesTo narrows a section or check to the languages, addons and/or
// stages it is relevant for. Every slice is optional; an empty
// AppliesTo (the zero value) matches everything — the resolver (phase
// 13) owns that interpretation, this package only carries the data.
type AppliesTo struct {
	Languages []string `json:"languages,omitempty"`
	Addons    []string `json:"addons,omitempty"`
	Stages    []string `json:"stages,omitempty"`
}

// Section is one standards.json sections[] entry: one guidance file.
type Section struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Severity  string    `json:"severity"`
	AppliesTo AppliesTo `json:"applies_to,omitempty"`
}

// Check is one standards.json checks[] entry: one tool run the CLI
// performs, once internal/standards/check runners exist (phase 14).
type Check struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Tool      string    `json:"tool"`
	Config    string    `json:"config,omitempty"`
	Severity  string    `json:"severity"`
	Input     string    `json:"input,omitempty"` // "source" (default) or "render"
	AppliesTo AppliesTo `json:"applies_to,omitempty"`
}

// Manifest is one pack's standards.json, unmarshalled.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Version       string    `json:"version"`
	Sections      []Section `json:"sections"`
	Checks        []Check   `json:"checks"`
}

// Load reads dir/standards.json, validates it against the embedded
// "pack" schema, and unmarshals it. A missing manifest, malformed JSON
// or a schema mismatch are all *envelope.Error{Code: "pack_invalid"} —
// the caller (Validate, or a CLI command reading a pack directly) never
// needs to distinguish "file absent" from "file wrong shape" itself.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFile)
	data, err := os.ReadFile(path) //nolint:gosec // G304: dir is explicit CLI input, not attacker-controlled
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &envelope.Error{Code: "pack_invalid", Message: fmt.Sprintf("%s: no %s", dir, ManifestFile)}
		}
		return nil, fmt.Errorf("standards: read %s: %w", path, err)
	}

	if err := schema.Validate("pack", data); err != nil {
		return nil, &envelope.Error{Code: "pack_invalid", Message: fmt.Sprintf("%s: manifest does not match the pack schema", path), Details: []string{err.Error()}}
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		// schema.Validate already rejects malformed JSON above, so a
		// failure here would be this function's own bug, not caller
		// input — 04-errors.md rule 9's must* shape without the panic,
		// since Load's contract is "return an error," not "never fail."
		return nil, fmt.Errorf("standards: %s passed schema validation but did not unmarshal: %w", path, err)
	}
	return &m, nil
}
