package ops

// Rendered is one target materialized from a git ref (branch or base)
// by Addon.Render — the real, unredacted rendering internal/ops/render
// (phase 19) memoizes by tree hash and only Exec ever reads. Nothing
// in this phase constructs one for real; the shape exists now so the
// Addon interface compiles and the fixture addon has something to
// return.
type Rendered struct {
	// Dir is the real rendering's own directory
	// (~/.ai-flow/cache/render/<hash>/ — phase 19 owns the path).
	Dir string
}

// Finding is one Addon.Validate result — a validator's own diagnostic
// against a Rendered target, independent of internal/standards/check's
// Finding (that shape is for standards checks; this one is for a
// tool's own static validation of a rendering, e.g. tflint, kubeconform).
type Finding struct {
	Tool     string
	Severity string
	Message  string
	File     string
	Line     int
}
