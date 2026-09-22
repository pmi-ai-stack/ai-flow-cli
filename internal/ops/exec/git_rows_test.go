package exec

import "testing"

// TestGitRowsClassAndMutating pins the phase-5 plan's design decision
// 15: every git row is class repo_write, and exactly the local-mutation
// and publishing verbs (fetch, worktree add, checkout, add, commit,
// push) are Mutating: true — the read-only helpers (status, rev-parse,
// rev-list, merge-base, remote get-url, diff) are not.
func TestGitRowsClassAndMutating(t *testing.T) {
	wantMutating := map[string]bool{
		"status":         false,
		"rev-parse":      false,
		"rev-list":       false,
		"merge-base":     false,
		"remote get-url": false,
		"diff":           false,
		"ls-files":       false,
		"fetch":          true,
		"worktree add":   true,
		"checkout":       true,
		"add":            true,
		"commit":         true,
		"push":           true,
	}
	found := map[string]bool{}
	for _, row := range Table {
		if row.Tool != "git" {
			continue
		}
		key := ""
		for i, v := range row.Verb {
			if i > 0 {
				key += " "
			}
			key += v
		}
		found[key] = true
		want, ok := wantMutating[key]
		if !ok {
			t.Errorf("unexpected git row %q in Table", key)
			continue
		}
		if row.Mutating != want {
			t.Errorf("git %s: Mutating = %v, want %v", key, row.Mutating, want)
		}
		if row.Class != ClassRepoWrite {
			t.Errorf("git %s: Class = %q, want %q", key, row.Class, ClassRepoWrite)
		}
	}
	for verb := range wantMutating {
		if !found[verb] {
			t.Errorf("expected git row %q not found in Table", verb)
		}
	}
}

func TestGitCommandHintRegistered(t *testing.T) {
	if _, ok := CommandHints["git"]; !ok {
		t.Error(`CommandHints["git"] not set — the exec_not_allowed denial for a bare git call would name no equivalent command`)
	}
}
