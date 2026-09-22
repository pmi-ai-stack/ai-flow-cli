package evals

import (
	"testing"
)

func TestExtractKey(t *testing.T) {
	cases := []struct {
		query  string
		want   string
		wantOK bool
	}{
		{"develop PROJ-1", "PROJ-1", true},
		{"run mutation for PROJ-1", "PROJ-1", true},
		{"develop PROJ-1, skip the plan gate", "PROJ-1", true},
		{"ABC-123 needs review", "ABC-123", true},
		{"no key here", "", false},
		{"lowercase-1 is not a key", "", false},
	}
	for _, c := range cases {
		got, ok := ExtractKey(c.query)
		if got != c.want || ok != c.wantOK {
			t.Errorf("ExtractKey(%q) = (%q, %v), want (%q, %v)", c.query, got, ok, c.want, c.wantOK)
		}
	}
}
