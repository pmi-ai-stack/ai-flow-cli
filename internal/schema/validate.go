package schema

import (
	"encoding/json"
	"fmt"
)

// Validate checks data against artifact's embedded schema. artifact must
// be one of Artifacts(); an unknown name returns ErrUnknownArtifact.
// Malformed JSON or a schema mismatch both return *ValidationError, so a
// caller that only cares "is this artifact valid" has one error type to
// branch on with errors.As.
func Validate(artifact string, data []byte) error {
	if !knownArtifact(artifact) {
		return fmt.Errorf("%w: %q", ErrUnknownArtifact, artifact)
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return &ValidationError{
			Artifact: artifact,
			Failures: []Failure{{Pointer: "", Message: "invalid JSON: " + err.Error()}},
		}
	}

	s := compiled[artifact]()
	if err := s.Validate(v); err != nil {
		return newValidationError(artifact, err)
	}
	return nil
}

// Print returns the embedded schema document itself (not an instance to
// validate), decoded into a generic value so the caller can marshal it
// back out unchanged — used by `ai-flow schema <artifact>`.
func Print(artifact string) (any, error) {
	if !knownArtifact(artifact) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownArtifact, artifact)
	}

	data, err := rawSchemaBytes(artifact)
	if err != nil {
		return nil, err
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		panic(fmt.Sprintf("schema: embedded %s is not valid JSON: %v", filename(artifact), err))
	}
	return v, nil
}
