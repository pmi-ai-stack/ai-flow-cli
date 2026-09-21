package schema

import (
	"errors"
	"slices"
)

// ErrUnknownArtifact is returned by Validate and Print when the artifact
// name is not one of Artifacts().
var ErrUnknownArtifact = errors.New("schema: unknown artifact")

// artifacts is every artifact name the spec's handoff table names, in
// the order that table lists them. This is the full enumeration: an
// artifact missing from this list has no embedded schema at all, which
// TestAllHandoffArtifactsHaveASchema in this package's tests guards.
var artifacts = []string{
	"status",
	"standards",
	"ticket",
	"repo",
	"ops-targets",
	"test-results",
	"render-drift-results",
	"dryrun-results",
	"impact-results",
	"rollback-results",
	"infra-test-results",
	"checks-results",
	"mutation-results",
	"visual-test-results",
	"e2e-results",
	"ship-results",
	"skill-frontmatter",
}

// Artifacts returns every embedded schema's artifact name, in table
// order. The caller's to keep — always a fresh copy.
func Artifacts() []string {
	return slices.Clone(artifacts)
}

// knownArtifact reports whether name is one of Artifacts().
func knownArtifact(name string) bool {
	return slices.Contains(artifacts, name)
}

// filename returns the embedded file name for an artifact, without
// checking that the artifact is known — callers that care use
// knownArtifact first.
func filename(artifact string) string {
	return artifact + ".schema.json"
}
