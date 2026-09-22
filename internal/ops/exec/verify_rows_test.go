package exec

import "testing"

// TestVerifyRunnerRowsClassAndMutating pins the plan's design decision
// 8: the four unit/e2e runner rows this phase adds are all
// Class: none, Mutating: false — the table's first non-repo_write rows.
func TestVerifyRunnerRowsClassAndMutating(t *testing.T) {
	type key struct {
		tool string
		verb string
	}
	want := map[key]bool{
		{"go", "test"}:             false,
		{"npx", "jest"}:            false,
		{"pytest", ""}:             false,
		{"npx", "playwright test"}: false,
	}
	found := map[key]bool{}
	for _, row := range Table {
		verbKey := ""
		for i, v := range row.Verb {
			if i > 0 {
				verbKey += " "
			}
			verbKey += v
		}
		k := key{row.Tool, verbKey}
		wantMutating, ok := want[k]
		if !ok {
			// Not one of this phase's own four rows (a later phase's own
			// npx/go/pytest row, e.g. phase 7's "npx stryker run" —
			// pinned by its own TestVerifyMutationRowsClassAndMutating
			// instead).
			continue
		}
		found[k] = true
		if row.Mutating != wantMutating {
			t.Errorf("%s %v: Mutating = %v, want %v", row.Tool, row.Verb, row.Mutating, wantMutating)
		}
		if row.Class != ClassNone {
			t.Errorf("%s %v: Class = %q, want %q", row.Tool, row.Verb, row.Class, ClassNone)
		}
	}
	for k := range want {
		if !found[k] {
			t.Errorf("expected row %+v not found in Table", k)
		}
	}
}

func TestVerifyRunnerCommandHintsRegistered(t *testing.T) {
	for _, tool := range []string{"go", "npx", "pytest"} {
		if _, ok := CommandHints[tool]; !ok {
			t.Errorf("CommandHints[%q] not set — the exec_not_allowed denial would name no equivalent command", tool)
		}
	}
}
