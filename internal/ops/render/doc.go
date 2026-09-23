// Package render is the real/redacted split every later OPS phase
// depends on (spec's Determinism §6): materializing one ops.Target at
// one ops.Ref through its ops.Addon, memoized by content-addressed
// tree hash. The real rendering lives at CachePath(home, hash), mode
// 0700, outside any ticket directory and never named by any artifact;
// the redacted copy — built via internal/redact.RedactMarked over the
// tool's own sensitivity markings (ops.Rendered.Marked) — lives under
// the calling ticket's own render/ directory, which is what the agent
// and every later OPS stage reads.
package render
