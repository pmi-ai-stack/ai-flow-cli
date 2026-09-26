package lint

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// TestCheckInvocation_ThreeWordCommandName proves checkInvocation
// matches a three-word Catalog() entry ("ops test detect"/"ops test
// run"/"ops test teardown", the only such entries today) rather than
// falling back to a nonexistent two-word or one-word prefix — the
// phase-28 regression: checkInvocation used to only ever try two-word
// and one-word catalog names, so any skill invoking one of these three
// commands failed unknown-command regardless of correct usage.
func TestCheckInvocation_ThreeWordCommandName(t *testing.T) {
	s := skills.Skill{Name: "test-infra"}
	for _, tc := range []struct {
		name       string
		invocation string
		wantRule   string // "" means no finding
	}{
		{"detect, valid", "ai-flow ops test detect KEY", ""},
		{"run, valid with flags", "ai-flow ops test run KEY --harness NAME --ref base", ""},
		{"teardown, valid", "ai-flow ops test teardown KEY", ""},
		{"run, unrecognized flag", "ai-flow ops test run KEY --bogus x", RuleArgOrder},
		{"detect, missing key", "ai-flow ops test detect", RuleArgOrder},
		{"unknown three-word command", "ai-flow ops test frobnicate KEY", RuleUnknownCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			findings := checkInvocation(s, tc.invocation)
			if tc.wantRule == "" {
				if len(findings) != 0 {
					t.Fatalf("checkInvocation(%q) = %+v, want no findings", tc.invocation, findings)
				}
				return
			}
			if len(findings) != 1 || findings[0].Rule != tc.wantRule {
				t.Fatalf("checkInvocation(%q) = %+v, want exactly one finding with Rule %q", tc.invocation, findings, tc.wantRule)
			}
		})
	}
}
