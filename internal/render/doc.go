// Package render builds output.md and the PR body's generated summary
// block from a ticket's own handoff artifacts. Unlike every other
// package built so far, it owns no stage and no artifact schema of its
// own (output.md has none — it is Markdown, not one of the 17
// schema-registered JSON artifacts); it only reads across what earlier
// stages already wrote (repo.json, mutation-results.json) and renders
// it deterministically. Checks, standards packs and waivers are
// rendered as a fixed placeholder until internal/standards exists
// (phase 12-14); the OPS section (environments, drift, changeset,
// score, blast radius, rollback, infra tests) is omitted entirely
// until internal/ops exists (phase 18-24) — there is nothing to read
// yet, not an empty section to fill.
package render
