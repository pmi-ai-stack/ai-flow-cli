// Package handoff is the pipeline's spine: the home directory layout,
// the byte-stable atomic JSON writer every artifact writer in this
// module builds on, the sixteen-stage state machine over
// status.json, active.json and each ticket's events.jsonl, and the
// two lookup tables (Catalog, StageTable) that `status`, `skills lint`
// (phase 10) and `doctor` (phase 8) all read.
//
// This package never calls a provider (tracker, git, host, an OPS
// tool) and never writes any artifact besides status.json, active.json
// and events.jsonl — every other artifact named in the spec's handoff
// table is only ever read here (existence and schema checks a stage's
// own completion performs), and only written by the phase that owns
// it.
package handoff
