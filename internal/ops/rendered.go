package ops

import "github.com/pmi-ai-stack/ai-flow-cli/internal/redact"

// Rendered is one target materialized from a git ref (branch or base)
// by Addon.Render — the real, unredacted rendering internal/ops/render
// (phase 19) memoizes by tree hash and reads to build the ticket's own
// redacted copy. Nothing before phase 19 constructs one for real; the
// shape existed from phase 18 on so the Addon interface compiles and
// the fixture addon has something to return.
type Rendered struct {
	// Dir is the real rendering's own directory
	// (~/.ai-flow/cache/render/<hash>/ — internal/ops/render owns the
	// path).
	Dir string
	// Marked is every value in this rendering the tool itself marked
	// sensitive (a Kubernetes Secret's data, a Terraform attribute the
	// provider schema marks sensitive) — what internal/ops/render feeds
	// to redact.RedactMarked to build the redacted copy the agent
	// reads. Empty for a rendering with nothing sensitive in it.
	Marked []redact.MarkedValue
}

// Finding is one Addon.Validate result — a validator's own diagnostic
// against a Rendered target, independent of internal/standards/check's
// Finding (that shape is for standards checks; this one is for a
// tool's own static validation of a rendering, e.g. tflint, kubeconform).
// Fields are declared in JSON-key alphabetical order (target.go's own
// documented byte-stability convention) — phase 20 is this type's first
// real serializer (render-drift-results.json's and dryrun-results.json's
// own "validation[]").
type Finding struct {
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Tool     string `json:"tool"`
}
