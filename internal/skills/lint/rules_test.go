package lint

import (
	"path/filepath"
	"testing"
)

// allRuleIDs is every rule id this package produces, declared once and
// shared by TestLint_InvalidFixtures (which needs the list to find each
// fixture) and TestRuleCount (which only needs its length) — a single
// list to keep in sync instead of two.
var allRuleIDs = []string{
	RuleFrontmatterNotFirstLine,
	RuleNameMatchesDirectory,
	RuleDescriptionForm,
	RuleFrontmatterInvalid,
	RuleDisableModelInvocationMisplaced,
	RuleFrontmatterUnknownField,
	RuleNonPortableToken,
	RuleUnknownCommand,
	RuleArgOrder,
	RuleUnknownStageID,
	RuleMissingReference,
	RuleReferenceTooLong,
	RuleReferenceNotIdentical,
	RuleDirectoryNotAllowed,
	RuleBodyTooLong,
	RuleMissingEvals,
	RuleTerminology,
	RuleAddonSectionMissing,
	RuleAddonSectionMismatch,
}

// TestLint_InvalidFixtures runs Lint against every testdata/skills/
// invalid-<rule> fixture and asserts that the rule its name states
// fired, and that no *other* rule fired — the phase's own "Done when"
// bar ("fails each invalid-* with exactly its rule id").
func TestLint_InvalidFixtures(t *testing.T) {
	for _, rule := range allRuleIDs {
		t.Run(rule, func(t *testing.T) {
			root := filepath.Join("..", "..", "..", "testdata", "skills", "invalid-"+rule)
			res, err := Lint(root)
			if err != nil {
				t.Fatalf("Lint(%s): %v", root, err)
			}
			if len(res.Findings) == 0 {
				t.Fatalf("Lint(%s) found no findings, want at least one with Rule %q", root, rule)
			}
			for _, f := range res.Findings {
				if f.Rule != rule {
					t.Errorf("Lint(%s) found unexpected rule %q (%+v), want only %q", root, f.Rule, f, rule)
				}
			}
		})
	}
}

// TestRuleCount pins the number of distinct rule ids this package
// produces, so a future rule addition or removal touches this test
// deliberately rather than silently changing the fixture count this
// phase's own "one invalid-<rule> tree per rule" bar depends on.
func TestRuleCount(t *testing.T) {
	if len(allRuleIDs) != 19 {
		t.Fatalf("got %d rule ids, want 19", len(allRuleIDs))
	}
}
