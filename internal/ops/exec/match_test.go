package exec

import (
	"strings"
	"testing"
)

func TestVerbMatchingOneWord(t *testing.T) {
	al := NewAllowlist(Row{
		Tool:  "kubectl",
		Verb:  []string{"diff"},
		Flags: []string{"-f"},
		Class: ClassReadOnlyLive,
	})

	if _, ok := al.Match([]string{"kubectl", "diff", "-f", "manifest.yaml"}); !ok {
		t.Error("kubectl diff -f manifest.yaml should match the diff row")
	}
	if _, ok := al.Match([]string{"kubectl", "apply", "-f", "manifest.yaml"}); ok {
		t.Error("kubectl apply should not match the diff row")
	}
	if _, ok := al.Match([]string{"helm", "diff"}); ok {
		t.Error("a different tool with the same verb should not match")
	}
}

// TestVerbMatchingTwoWord proves two-word verbs (e.g. "rollout history")
// match on both words, and that free arguments after the verb are not
// matched by value — "helm diff upgrade" is the verb "diff", per the
// spec's own example.
func TestVerbMatchingTwoWord(t *testing.T) {
	al := NewAllowlist(
		Row{Tool: "kubectl", Verb: []string{"rollout", "history"}, Class: ClassReadOnlyLive},
		Row{Tool: "helm", Verb: []string{"diff"}, Class: ClassNone},
	)

	if _, ok := al.Match([]string{"kubectl", "rollout", "history", "deployment/web"}); !ok {
		t.Error("kubectl rollout history <free arg> should match")
	}
	if _, ok := al.Match([]string{"kubectl", "rollout", "restart", "deployment/web"}); ok {
		t.Error("kubectl rollout restart should not match the rollout-history row")
	}
	if _, ok := al.Match([]string{"kubectl", "rollout"}); ok {
		t.Error("a two-word verb with only the first word present should not match")
	}
	if _, ok := al.Match([]string{"helm", "diff", "upgrade", "myrelease", "./chart"}); !ok {
		t.Error(`"helm diff upgrade …" is the verb "diff" — free arguments after it are not matched by value`)
	}
}

func TestVerbMatchingRequiredAndForbiddenFlags(t *testing.T) {
	al := NewAllowlist(
		Row{
			Tool:          "kubectl",
			Verb:          []string{"apply"},
			RequiredFlags: []string{"--dry-run=server"},
			Flags:         []string{"-f"},
			Mutating:      true,
			Class:         ClassReadOnlyLive,
		},
		Row{
			Tool:           "terraform",
			Verb:           []string{"plan"},
			ForbiddenFlags: []string{"-auto-approve"},
			Class:          ClassNone,
		},
	)

	if _, ok := al.Match([]string{"kubectl", "apply", "-f", "x.yaml"}); ok {
		t.Error("kubectl apply without the required dry-run flag should not match")
	}
	if _, ok := al.Match([]string{"kubectl", "apply", "-f", "x.yaml", "--dry-run=server"}); !ok {
		t.Error("kubectl apply with the required dry-run flag should match")
	}
	if _, ok := al.Match([]string{"terraform", "plan", "-auto-approve"}); ok {
		t.Error("terraform plan -auto-approve should not match (forbidden flag present)")
	}
	if _, ok := al.Match([]string{"terraform", "plan"}); !ok {
		t.Error("terraform plan without the forbidden flag should match")
	}
}

func TestVerbMatchingEmptyArgv(t *testing.T) {
	al := NewAllowlist(Row{Tool: "git", Verb: []string{"push"}, Class: ClassRepoWrite})
	if _, ok := al.Match(nil); ok {
		t.Error("empty argv should never match")
	}
	if _, ok := al.Match([]string{}); ok {
		t.Error("empty argv should never match")
	}
}

// withCommandHints swaps the package-level CommandHints for hints for
// the duration of t, restoring the real value on cleanup — every test
// in this package that reads CommandHints (git_rows_test.go,
// verify_rows_test.go) depends on it surviving whichever test in this
// file ran first, so a bare reset to an empty map here would corrupt
// every later test's view of it.
func withCommandHints(t *testing.T, hints map[string]string) {
	t.Helper()
	original := CommandHints
	CommandHints = hints
	t.Cleanup(func() { CommandHints = original })
}

func TestExecNotAllowedNamesCommand(t *testing.T) {
	withCommandHints(t, map[string]string{"terraform": "ops dry-run KEY --target T"})

	err := NotAllowed([]string{"terraform", "apply"})
	if err.Code != "exec_not_allowed" {
		t.Errorf("code = %q, want exec_not_allowed", err.Code)
	}
	if !strings.Contains(err.Message, "terraform apply") {
		t.Errorf("message %q does not name the denied command", err.Message)
	}
	if !strings.Contains(err.Message, "ops dry-run KEY --target T") {
		t.Errorf("message %q does not name the equivalent ai-flow command", err.Message)
	}
}

func TestExecNotAllowedNoHint(t *testing.T) {
	withCommandHints(t, map[string]string{})
	err := NotAllowed([]string{"some-random-tool", "do-a-thing"})
	if err.Code != "exec_not_allowed" {
		t.Errorf("code = %q, want exec_not_allowed", err.Code)
	}
	if !strings.Contains(err.Message, "some-random-tool do-a-thing") {
		t.Errorf("message %q does not name the denied command", err.Message)
	}
}
