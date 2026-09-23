package standards

import (
	"slices"
	"testing"
)

// TestBudgetDropOrder is the spec's "budget drop order": `may` drops
// first, then `should`, never `must`; within a severity, `repo` before
// `team` before `org`; within a layer, by id.
func TestBudgetDropOrder(t *testing.T) {
	// Every candidate is 400 bytes = 100 tokens, so a budget of N*100
	// keeps exactly N entries and the arithmetic stays obvious.
	c := func(id, layer, severity string) budgetCandidate {
		return budgetCandidate{id: id, layer: layer, severity: severity, bytes: 400}
	}
	candidates := []budgetCandidate{
		c("z.may", LayerOrg, "may"),
		c("a.should.org", LayerOrg, "should"),
		c("a.should.repo", LayerRepo, "should"),
		c("b.must", LayerOrg, "must"),
	}

	tests := []struct {
		name        string
		budget      int
		wantKept    []string
		wantDropped []string
	}{
		{
			name:     "unbudgeted keeps everything",
			budget:   0,
			wantKept: []string{"b.must", "a.should.repo", "a.should.org", "z.may"}, wantDropped: []string{},
		},
		{
			name:     "room for everything",
			budget:   400,
			wantKept: []string{"b.must", "a.should.repo", "a.should.org", "z.may"}, wantDropped: []string{},
		},
		{
			name:     "may drops first",
			budget:   300,
			wantKept: []string{"b.must", "a.should.repo", "a.should.org"}, wantDropped: []string{"z.may"},
		},
		{
			name:     "then the lowest-priority should",
			budget:   200,
			wantKept: []string{"b.must", "a.should.repo"}, wantDropped: []string{"a.should.org", "z.may"},
		},
		{
			name:     "repo should outlives org should",
			budget:   100,
			wantKept: []string{"b.must"}, wantDropped: []string{"a.should.repo", "a.should.org", "z.may"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := selectForStage(candidates, tc.budget)
			if !slices.Equal(got.Kept, tc.wantKept) {
				t.Errorf("Kept = %v, want %v", got.Kept, tc.wantKept)
			}
			if !slices.Equal(got.Dropped, tc.wantDropped) {
				t.Errorf("Dropped = %v, want %v", got.Dropped, tc.wantDropped)
			}
			if got.MustOverflowBytes != 0 {
				t.Errorf("MustOverflowBytes = %d, want 0", got.MustOverflowBytes)
			}
		})
	}
}

// TestBudgetNeverKeepsAMayOverAShould is the property the tail-drop
// implementation exists for: a small `may` must not survive a budget
// that dropped a larger `should` earlier in the order.
func TestBudgetNeverKeepsAMayOverAShould(t *testing.T) {
	got := selectForStage([]budgetCandidate{
		{id: "big.should", layer: LayerOrg, severity: "should", bytes: 4000}, // 1000 tokens
		{id: "tiny.may", layer: LayerOrg, severity: "may", bytes: 4},         // 1 token
	}, 100)

	if slices.Contains(got.Kept, "tiny.may") {
		t.Errorf("Kept = %v: a may survived a dropped should, which inverts severity", got.Kept)
	}
	if !slices.Equal(got.Dropped, []string{"big.should", "tiny.may"}) {
		t.Errorf("Dropped = %v, want both, should first", got.Dropped)
	}
}

// TestBudgetMustOverflowKeepsEveryMust is the spec's documented
// overflow behaviour: "every must section is kept, `Budget exceeded by
// must sections:` records the overflow in bytes". Nothing truncates a
// must, and no should or may can fit below one.
func TestBudgetMustOverflowKeepsEveryMust(t *testing.T) {
	got := selectForStage([]budgetCandidate{
		{id: "a.must", layer: LayerOrg, severity: "must", bytes: 4000},   // 1000 tokens
		{id: "b.must", layer: LayerRepo, severity: "must", bytes: 4000},  // 1000 tokens
		{id: "c.should", layer: LayerOrg, severity: "should", bytes: 40}, // 10 tokens
	}, 500)

	if !slices.Equal(got.Kept, []string{"b.must", "a.must"}) {
		t.Errorf("Kept = %v, want both musts (repo before org), and nothing else", got.Kept)
	}
	if !slices.Equal(got.Dropped, []string{"c.should"}) {
		t.Errorf("Dropped = %v, want [c.should]", got.Dropped)
	}
	// 8000 bytes of must against a 500-token (2000-byte) budget.
	if got.MustOverflowBytes != 6000 {
		t.Errorf("MustOverflowBytes = %d, want 6000", got.MustOverflowBytes)
	}
}

// TestTokensForIsBytesOverFour pins the spec's own approximation, the
// one that makes the selection byte-stable rather than model-dependent.
func TestTokensForIsBytesOverFour(t *testing.T) {
	for _, tc := range []struct{ bytes, want int }{{0, 0}, {3, 0}, {4, 1}, {7, 1}, {4000, 1000}} {
		if got := tokensFor(tc.bytes); got != tc.want {
			t.Errorf("tokensFor(%d) = %d, want %d", tc.bytes, got, tc.want)
		}
	}
}

// TestBudgetOrderIsTotal proves the three-key ordering is a strict
// total order over distinct entries, which is what makes the selection
// a pure function rather than a sort-stability accident.
func TestBudgetOrderIsTotal(t *testing.T) {
	ordered := []budgetCandidate{
		{id: "a", layer: LayerRepo, severity: "must"},
		{id: "b", layer: LayerRepo, severity: "must"},
		{id: "a", layer: LayerTeam, severity: "must"},
		{id: "a", layer: LayerOrg, severity: "must"},
		{id: "a", layer: LayerRepo, severity: "should"},
		{id: "a", layer: LayerOrg, severity: "should"},
		{id: "a", layer: LayerRepo, severity: "may"},
	}
	for i := range ordered[:len(ordered)-1] {
		if budgetOrder(ordered[i], ordered[i+1]) >= 0 {
			t.Errorf("budgetOrder(%+v, %+v) >= 0, want a strict ordering", ordered[i], ordered[i+1])
		}
	}
}
