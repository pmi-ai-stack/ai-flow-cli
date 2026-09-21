package exec

import "testing"

// defaultRows is the table this binary actually ships. It is empty in
// phase 1 — every later phase appends its own rows here, in the same PR
// that needs them, and TestAllowlistNeverMutatesPersistentState runs
// against whatever is here.
var defaultRows []Row

// TestAllowlistNeverMutatesPersistentState is the invariant: no row can
// change a persistent infrastructure target. Every row with
// Mutating: true must be in exactly one of three states — its own
// dry-run flag in RequiredFlags (class none or read_only_live), class
// ephemeral, or class repo_write. It runs vacuously today, against zero
// rows, so a future row that violates it fails the build the moment it
// is added rather than being discovered later.
func TestAllowlistNeverMutatesPersistentState(t *testing.T) {
	testAllowlistNeverMutatesPersistentState(t, defaultRows)
}

// errorfer is the subset of *testing.T the invariant check needs. A
// concrete interface (not *testing.T) lets
// TestAllowlistNeverMutatesPersistentStateCatchesViolations drive the
// same check against a recorder instead of failing the outer test.
type errorfer interface {
	Helper()
	Errorf(format string, args ...any)
}

func testAllowlistNeverMutatesPersistentState(t errorfer, rows []Row) {
	t.Helper()
	for _, r := range rows {
		if !r.Mutating {
			continue
		}
		switch r.Class {
		case ClassEphemeral, ClassRepoWrite:
			// Both classes are documented as safe for mutating rows:
			// ephemeral acts only on a cluster/workspace the CLI itself
			// created and will delete; repo_write's publishing verbs
			// are reachable only from `git commit`/`git push`, gated on
			// a recorded ship approval.
			continue
		case ClassNone, ClassReadOnlyLive:
			if len(r.RequiredFlags) == 0 {
				t.Errorf("row %s %v: mutating, class %s, but carries no dry-run flag in RequiredFlags", r.Tool, r.Verb, r.Class)
			}
		default:
			t.Errorf("row %s %v: unknown class %q", r.Tool, r.Verb, r.Class)
		}
	}
}

// TestAllowlistNeverMutatesPersistentStateCatchesViolations proves the
// invariant test above is not vacuously true by construction — it would
// catch a bad row if one were ever added.
func TestAllowlistNeverMutatesPersistentStateCatchesViolations(t *testing.T) {
	cases := []struct {
		name string
		row  Row
	}{
		{
			name: "mutating, class none, no dry-run flag",
			row:  Row{Tool: "terraform", Verb: []string{"apply"}, Mutating: true, Class: ClassNone},
		},
		{
			name: "mutating, class read_only_live, no dry-run flag",
			row:  Row{Tool: "kubectl", Verb: []string{"apply"}, Mutating: true, Class: ClassReadOnlyLive},
		},
		{
			name: "mutating, unknown class",
			row:  Row{Tool: "helm", Verb: []string{"upgrade"}, Mutating: true, Class: Class("mystery")},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &recordingT{T: t}
			testAllowlistNeverMutatesPersistentState(rt, []Row{c.row})
			if !rt.errored {
				t.Errorf("expected the invariant test to reject row %+v, it passed", c.row)
			}
		})
	}
}

// recordingT wraps *testing.T so a sub-test can prove the invariant
// helper reports failures without failing the outer test itself.
type recordingT struct {
	*testing.T
	errored bool
}

func (r *recordingT) Errorf(format string, args ...any) {
	r.errored = true
}

func TestAllowlistNeverMutatesPersistentStateAllowsValidRows(t *testing.T) {
	rows := []Row{
		{Tool: "kubectl", Verb: []string{"apply"}, RequiredFlags: []string{"--dry-run=server"}, Mutating: true, Class: ClassReadOnlyLive},
		{Tool: "act", Verb: nil, RequiredFlags: []string{"-n"}, Mutating: true, Class: ClassNone},
		{Tool: "kind", Verb: []string{"create", "cluster"}, Mutating: true, Class: ClassEphemeral},
		{Tool: "git", Verb: []string{"push"}, Mutating: true, Class: ClassRepoWrite},
		{Tool: "terraform", Verb: []string{"plan"}, Mutating: false, Class: ClassNone},
	}
	rt := &recordingT{T: t}
	testAllowlistNeverMutatesPersistentState(rt, rows)
	if rt.errored {
		t.Error("valid rows were rejected by the invariant test")
	}
}
