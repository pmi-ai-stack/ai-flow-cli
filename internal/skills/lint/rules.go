package lint

import "strings"

// Rule ids. Fifteen are named directly by ai-flow/docs/skill-authoring.md's
// own table; RuleFrontmatterInvalid and RuleFrontmatterNotFirstLine are
// assigned by this phase for the three "enforced by the schema" concerns
// and the one prose-only rule that table leaves without an id of their
// own — see the plan's design decision 1.
const (
	RuleFrontmatterNotFirstLine         = "frontmatter-not-first-line"
	RuleNameMatchesDirectory            = "name-matches-directory"
	RuleDescriptionForm                 = "description-form"
	RuleFrontmatterInvalid              = "frontmatter-invalid"
	RuleDisableModelInvocationMisplaced = "disable-model-invocation-misplaced"
	RuleFrontmatterUnknownField         = "frontmatter-unknown-field"
	RuleNonPortableToken                = "non-portable-token"
	RuleUnknownCommand                  = "unknown-command"
	RuleArgOrder                        = "arg-order"
	RuleUnknownStageID                  = "unknown-stage-id"
	RuleMissingReference                = "missing-reference"
	RuleReferenceTooLong                = "reference-too-long"
	RuleReferenceNotIdentical           = "reference-not-identical"
	RuleDirectoryNotAllowed             = "directory-not-allowed"
	RuleBodyTooLong                     = "body-too-long"
	RuleMissingEvals                    = "missing-evals"
	RuleTerminology                     = "terminology"
	RuleAddonSectionMissing             = "addon-section-missing"
)

// entryPointNames is the two skills disable-model-invocation is required
// (true) on; every other skill must omit it. develop and ship are
// phase 15's own content — this package only knows their names, per the
// Portable skill profile table.
var entryPointNames = map[string]bool{"develop": true, "ship": true}

// isEntryPoint and isAddonSkill are the one place a skill's "kind" is
// classified from its name — every rule that needs to know (description
// form, disable-model-invocation placement, the addon section check)
// calls these instead of re-deriving the same map lookup or prefix test
// locally.
func isEntryPoint(skillName string) bool { return entryPointNames[skillName] }

func isAddonSkill(skillName string) bool { return strings.HasPrefix(skillName, "addon-") }

// bodyHardLimit is the hard line-count ceiling every skill body must
// stay under (RuleBodyTooLong). bodyWarnStage/bodyWarnEntry are the
// soft, non-failing thresholds surfaced as envelope warnings.
const (
	bodyHardLimit  = 500
	bodyWarnStage  = 150
	bodyWarnEntry  = 250
	referenceLimit = 80
)
