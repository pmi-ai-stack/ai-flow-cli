// Package strutil holds tiny string-slice helpers shared across
// packages that would otherwise each reimplement them — found during
// phase 5's /simplify pass, when the same dedup+sort logic turned up
// independently in internal/routing, internal/gitops, internal/cli and
// internal/handoff.
package strutil

import (
	"slices"
	"sort"
)

// DedupSorted returns a sorted copy of in with empty strings and
// duplicates removed.
func DedupSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return slices.Clip(out)
}
