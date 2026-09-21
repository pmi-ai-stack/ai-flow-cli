// Package redact keeps every secret ai-flow ever reads out of every
// artifact, log line and error message. Two independent sources feed
// it: environment-variable secrets (New, discovered from the process
// environment against the spec's "Secrets and live access" table) and
// tool-marked sensitive values (RedactMarked, the hook phase 19's
// rendering and plan writers call with a tool's own markings). Both
// produce the same token shape, <redacted:sha256-prefix>, via the one
// shared HashPrefix formula.
package redact
