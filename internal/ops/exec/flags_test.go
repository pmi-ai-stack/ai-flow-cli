package exec

import "testing"

// TestRequiredValueFlagCannotBeOverridden: a required "name=value" flag
// is not satisfied when another argument names the same flag with a
// different value — the tools ai-flow drives take the last occurrence,
// so `kubectl apply --dry-run=server --dry-run=none` is a real apply
// (2026-10-04 security review #5).
func TestRequiredValueFlagCannotBeOverridden(t *testing.T) {
	al := NewAllowlist(Table...)
	refused := [][]string{
		{"kubectl", "apply", "-f", ".", "--dry-run=server", "--dry-run=none"},
		{"kubectl", "apply", "-f", ".", "--dry-run=none", "--dry-run=server"},
		{"kubectl", "apply", "-f", ".", "--dry-run=server", "--dry-run", "none"},
		{"terraform", "init", "-backend=false", "-backend=true"},
	}
	for _, argv := range refused {
		if row, ok := al.Match(argv); ok {
			t.Errorf("%v matched row %+v; a contradicting value must not satisfy the required flag", argv, row)
		}
	}

	allowed := [][]string{
		{"kubectl", "apply", "-f", ".", "--dry-run=server", "-o", "json"},
		{"kubectl", "apply", "-f", ".", "--dry-run=server", "--dry-run=server"},
		{"terraform", "init", "-backend=false", "-input=false"},
	}
	for _, argv := range allowed {
		if _, ok := al.Match(argv); !ok {
			t.Errorf("%v should still match", argv)
		}
	}
}

// TestForbiddenFlagMatchesValueSpelling: a forbidden flag is also
// forbidden in its "flag=value" spelling and with either dash count —
// `kyverno apply --cluster=true` reaches a live cluster as surely as
// `--cluster`.
func TestForbiddenFlagMatchesValueSpelling(t *testing.T) {
	al := NewAllowlist(Table...)
	refused := [][]string{
		{"kyverno", "apply", "policy.yaml", "--cluster=true"},
		{"kyverno", "apply", "policy.yaml", "-cluster"},
		{"ruff", "check", "--fix=true", "."},
		{"eslint", "--fix=true", "."},
	}
	for _, argv := range refused {
		if row, ok := al.Match(argv); ok {
			t.Errorf("%v matched row %+v; a forbidden flag's =value spelling must be refused", argv, row)
		}
	}
	if _, ok := al.Match([]string{"kyverno", "apply", "policy.yaml", "--resource", "r.yaml"}); !ok {
		t.Error("kyverno apply without --cluster should still match")
	}
	if _, ok := al.Match([]string{"eslint", "--fix-dry-run", "."}); !ok {
		t.Error("a different flag sharing a prefix (--fix-dry-run) is not --fix")
	}
}
