package redact

import (
	"slices"
	"strings"
)

// MarkedValue is one value a tool itself marked sensitive — a
// Kubernetes Secret's data, a Terraform attribute the provider schema
// marks sensitive. Pointer is a label for diagnostics only (typically a
// JSON pointer into the rendering); RedactMarked does not parse it.
type MarkedValue struct {
	Pointer string
	Value   string
}

// RedactMarked replaces every MarkedValue's Value in text with its
// HashPrefix token. This is the hook phase 19's rendering and plan
// writers call with a tool's own sensitivity markings — independent of
// any environment-variable secret, so it works before a Redactor even
// exists. Longest values are substituted first, the same rule String
// applies.
func RedactMarked(text string, marked []MarkedValue) string {
	sorted := slices.Clone(marked)
	slices.SortFunc(sorted, func(a, b MarkedValue) int {
		return len(b.Value) - len(a.Value)
	})
	for _, m := range sorted {
		if m.Value == "" {
			continue
		}
		text = strings.ReplaceAll(text, m.Value, HashPrefix(m.Value))
	}
	return text
}
