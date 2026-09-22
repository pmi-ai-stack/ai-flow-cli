package exec

import "testing"

// TestVerifyMutationRowsClassAndMutating pins the plan's design
// decision F: the five internal/mutation tool rows this phase adds are
// all Class: none, Mutating: false — same reasoning as phase 6's
// runner rows (TestVerifyRunnerRowsClassAndMutating), which this test
// mirrors.
func TestVerifyMutationRowsClassAndMutating(t *testing.T) {
	type key struct {
		tool string
		verb string
	}
	want := map[key]bool{
		{"gremlins", "unleash"}:                             false,
		{"npx", "stryker run"}:                              false,
		{"mutmut", "run"}:                                   false,
		{"mutmut", "junitxml"}:                              false,
		{"mvn", "org.pitest:pitest-maven:mutationCoverage"}: false,
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

func TestVerifyMutationCommandHintsRegistered(t *testing.T) {
	for _, tool := range []string{"gremlins", "mutmut", "mvn"} {
		if _, ok := CommandHints[tool]; !ok {
			t.Errorf("CommandHints[%q] not set — the exec_not_allowed denial would name no equivalent command", tool)
		}
	}
}
