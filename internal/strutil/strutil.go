// Package strutil holds tiny string-slice helpers shared across
// packages that would otherwise each reimplement them — found during
// phase 5's /simplify pass, when the same dedup+sort logic turned up
// independently in internal/routing, internal/gitops, internal/cli and
// internal/handoff.
package strutil

import (
	"slices"
	"sort"
	"strings"
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

// HasPathSegment reports whether path contains any of segments as a
// whole "/"-separated element, not a substring match — so "tests"
// matches "app/tests/foo.py" but not "app/testset/foo.py".
func HasPathSegment(path string, segments ...string) bool {
	for _, part := range strings.Split(path, "/") {
		for _, want := range segments {
			if part == want {
				return true
			}
		}
	}
	return false
}
