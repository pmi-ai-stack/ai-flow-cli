// Package config loads and validates ~/.ai-flow/config.json: the full
// type set from the spec's Configuration section, an embedded template
// (config init's only copy), Load (read + decode + the fixed defaults),
// and Validate (every documented rule). No secret ever appears in
// Config — tracker and host tokens are read from the environment
// directly by the packages that need them (phases 4-5), never merged
// into this struct.
package config

// CurrentSchemaVersion is the only config.json schema_version this
// binary accepts. There are no configuration upgrades yet.
const CurrentSchemaVersion = 1
