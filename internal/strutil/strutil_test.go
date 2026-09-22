package strutil

import (
	"slices"
	"testing"
)

func TestDedupSorted(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, nil},
		{"already sorted, no dupes", []string{"a", "b"}, []string{"a", "b"}},
		{"dupes and out of order", []string{"b", "a", "b", "c", "a"}, []string{"a", "b", "c"}},
		{"drops empty strings", []string{"", "b", "", "a"}, []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DedupSorted(c.in)
			if !slices.Equal(got, c.want) {
				t.Errorf("DedupSorted(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestDedupSortedReturnsNilForNoResults(t *testing.T) {
	if got := DedupSorted(nil); got != nil {
		t.Errorf("DedupSorted(nil) = %v, want nil (callers that need [] rely on this and convert themselves)", got)
	}
	if got := DedupSorted([]string{""}); got != nil {
		t.Errorf("DedupSorted([\"\"]) = %v, want nil", got)
	}
}
