// Package schema is the loader for the embedded JSON Schemas in the
// top-level schema package: the registry of the 17 artifact names the
// spec's handoff table names, and Validate(artifact, bytes), the one
// entry point every future atomic-write path validates an artifact
// against before it is renamed into place.
package schema
