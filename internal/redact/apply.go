package redact

import (
	"slices"
	"strings"
)

// String replaces every discovered secret's value in s with its
// HashPrefix token. Longest values are substituted first so that one
// secret being a substring of another cannot leave a partial value
// exposed.
func (r *Redactor) String(s string) string {
	for _, sec := range r.byLengthDesc() {
		s = strings.ReplaceAll(s, sec.Value, HashPrefix(sec.Value))
	}
	return s
}

// Bytes is String for the byte-slice shape artifacts and captured tool
// output arrive in.
func (r *Redactor) Bytes(b []byte) []byte {
	return []byte(r.String(string(b)))
}

func (r *Redactor) byLengthDesc() []Secret {
	out := slices.Clone(r.secrets)
	slices.SortFunc(out, func(a, b Secret) int {
		return len(b.Value) - len(a.Value)
	})
	return out
}
