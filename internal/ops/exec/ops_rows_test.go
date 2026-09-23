package exec

import "testing"

// TestOpsRowsCoverCapabilityMatrixVerbs is phase 18's own "Done when"
// pinned as a test: every OPS addon-tool row this phase committed to
// (see table.go's own comment for what is deliberately absent, and
// why) is really on Table, under the class the Execution allowlist
// section names.
func TestOpsRowsCoverCapabilityMatrixVerbs(t *testing.T) {
	want := []struct {
		tool     string
		verb     []string
		class    Class
		mutating bool
	}{
		{"terraform", []string{"init"}, ClassNone, false},
		{"terraform", []string{"validate"}, ClassNone, false},
		{"tflint", nil, ClassNone, false},
		{"terraform", []string{"plan"}, ClassReadOnlyLive, false}, // matches either drift or dry-run row
		{"terraform", []string{"show"}, ClassNone, false},
		{"terraform", []string{"test"}, ClassEphemeral, true},
		{"terraform", []string{"init"}, ClassReadOnlyLive, false},                // phase 21: the real, backend-reading init
		{"terraform", []string{"workspace", "select"}, ClassReadOnlyLive, false}, // phase 21

		{"kubeconform", nil, ClassNone, false},
		{"kubectl", []string{"diff"}, ClassReadOnlyLive, false},
		{"kubectl", []string{"apply"}, ClassReadOnlyLive, true},
		{"kubectl", []string{"rollout", "history"}, ClassReadOnlyLive, false},
		{"kuttl", nil, ClassEphemeral, true},
		{"chainsaw", nil, ClassEphemeral, true},
		{"helm", []string{"template"}, ClassNone, false},
		{"helm", []string{"lint"}, ClassNone, false},
		{"helm", []string{"get", "manifest"}, ClassReadOnlyLive, false},
		{"helm", []string{"diff", "upgrade"}, ClassReadOnlyLive, false},
		{"helm", []string{"history"}, ClassReadOnlyLive, false},
		{"helm", []string{"unittest"}, ClassNone, false},
		{"ct", []string{"lint"}, ClassNone, false},
		{"ct", []string{"install"}, ClassEphemeral, true},
		{"kustomize", []string{"build"}, ClassNone, false},
		{"actionlint", nil, ClassNone, false},
		{"act", nil, ClassNone, true},
		{"jenkinsfile-runner", nil, ClassEphemeral, true},
	}

	for _, w := range want {
		found := false
		for _, r := range Table {
			if r.Tool != w.tool {
				continue
			}
			if len(r.Verb) != len(w.verb) {
				continue
			}
			match := true
			for i := range w.verb {
				if r.Verb[i] != w.verb[i] {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			if r.Class == w.class && r.Mutating == w.mutating {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no row for tool %q verb %v class %s mutating %v", w.tool, w.verb, w.class, w.mutating)
		}
	}
}

// TestActRowOrderingKeepsDryRunFirst asserts the two act rows are
// declared with the dry-run row (-n) before the ephemeral row (-j
// without -n) — table.go's own comment explains why the order matters:
// "act -j JOB -n" must match the dry row, not the ephemeral one.
func TestActRowOrderingKeepsDryRunFirst(t *testing.T) {
	row, ok := NewAllowlist(Table...).Match([]string{"act", "-j", "build", "-n"})
	if !ok {
		t.Fatal("act -j build -n did not match any row")
	}
	if row.Class != ClassNone || !row.Mutating {
		t.Errorf("act -j build -n matched %+v, want the dry-run row (class none, mutating)", row)
	}

	row, ok = NewAllowlist(Table...).Match([]string{"act", "-j", "build"})
	if !ok {
		t.Fatal("act -j build did not match any row")
	}
	if row.Class != ClassEphemeral {
		t.Errorf("act -j build matched %+v, want the ephemeral row", row)
	}
}
