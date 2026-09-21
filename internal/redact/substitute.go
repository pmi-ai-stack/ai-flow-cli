package redact

import (
	"slices"
	"strings"
)

// replaceLongestFirst replaces every non-empty value in values with its
// HashPrefix token, longest value first, so that one value being a
// substring of another cannot leave a partial value exposed. Shared by
// Redactor.String (values are discovered secrets) and RedactMarked
// (values are a tool's own sensitivity markings) — the two redaction
// paths differ only in where their values come from, not in how
// substitution works.
func replaceLongestFirst(text string, values []string) string {
	sorted := slices.Clone(values)
	slices.SortFunc(sorted, func(a, b string) int {
		return len(b) - len(a)
	})
	for _, v := range sorted {
		if v == "" {
			continue
		}
		text = strings.ReplaceAll(text, v, HashPrefix(v))
	}
	return text
}
