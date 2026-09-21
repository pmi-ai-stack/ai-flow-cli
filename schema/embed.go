// Package schema holds the ai-flow-cli embedded JSON Schemas — the
// documented shape of every machine-readable handoff artifact. This
// package embeds the raw *.schema.json files only; internal/schema
// builds the loader, the artifact registry and the Validate entry point
// on top of FS.
//
// The split exists because go:embed patterns cannot contain ".." and so
// cannot reach outside the directory of the file that declares them —
// internal/schema cannot embed a sibling top-level schema/ directory
// directly, so the content lives here and the logic lives there.
package schema

import "embed"

// FS embeds every schema/*.schema.json file, unmodified.
//
//go:embed *.schema.json
var FS embed.FS
