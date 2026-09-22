package lint

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// knownHandoffNames is every artifact name StageTable() lists for any
// stage, plus the reserved top-level ~/.ai-flow/ entries — the set a
// bare-filename-shaped code span must belong to, or be an unknown stage
// or file (the second half of rule 10; the first half, a "stage
// <argument>"'s literal value, lives in commands.go).
var knownHandoffNames = sync.OnceValue(func() map[string]bool {
	m := make(map[string]bool)
	for _, def := range handoff.StageTable() {
		for _, a := range def.Artifacts {
			m[a] = true
		}
	}
	for _, r := range handoff.ReservedNames {
		m[r] = true
	}
	return m
})

// handoffTokenPattern matches a code span shaped like a handoff
// artifact: a bare filename with one of the three handoff extensions,
// or a bare trailing-slash directory name (render/, screenshots/,
// plans/, ...).
var handoffTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.(json|md|jsonl)|/)$`)

func checkHandoffFileTokens(s skills.Skill) []Finding {
	known := knownHandoffNames()
	seen := map[string]bool{}
	var findings []Finding
	for _, span := range codeSpans(s.Body) {
		if strings.HasPrefix(span, "ai-flow ") || strings.HasPrefix(span, "references/") {
			continue
		}
		if !handoffTokenPattern.MatchString(span) || known[span] || seen[span] {
			continue
		}
		seen[span] = true
		findings = append(findings, Finding{
			Rule: RuleUnknownStageID, Skill: s.Name, File: "SKILL.md",
			Message: fmt.Sprintf("%q is not a handoff file or directory StageTable() names", span),
		})
	}
	return findings
}

// referenceLinkPattern finds a references/<file>.md path anywhere in
// the body — inside a code span or a Markdown link target, both of
// which contain this literal substring.
var referenceLinkPattern = regexp.MustCompile(`references/[A-Za-z0-9_-]+\.md`)

// nonPortablePatterns is rule 7's four forbidden shapes, applied to the
// body once every recognized code span and reference link has been
// blanked out (see the plan's design decision 5) — except bangBacktick,
// which must run on the raw body first, since blanking backtick spans
// would also delete the very "!`" sequence it looks for.
var (
	bangBacktick   = regexp.MustCompile("!`")
	dollarSign     = regexp.MustCompile(`\$`)
	atSign         = regexp.MustCompile(`@`)
	leadingSlash   = regexp.MustCompile(`(^|\s)/[A-Za-z]`)
	parentTraverse = regexp.MustCompile(`\.\./`)
)

func checkNonPortableTokens(s skills.Skill) []Finding {
	if bangBacktick.MatchString(s.Body) {
		return []Finding{{Rule: RuleNonPortableToken, Skill: s.Name, File: "SKILL.md", Message: "shell substitution syntax (!`...`) is not portable across hosts"}}
	}

	stripped := codeSpanPattern.ReplaceAllString(s.Body, "")
	stripped = referenceLinkPattern.ReplaceAllString(stripped, "")

	switch {
	case dollarSign.MatchString(stripped):
		return []Finding{{Rule: RuleNonPortableToken, Skill: s.Name, File: "SKILL.md", Message: "\"$\" (a host environment variable or template placeholder) is not portable across hosts"}}
	case atSign.MatchString(stripped):
		return []Finding{{Rule: RuleNonPortableToken, Skill: s.Name, File: "SKILL.md", Message: "\"@\" is not portable across hosts"}}
	case leadingSlash.MatchString(stripped):
		return []Finding{{Rule: RuleNonPortableToken, Skill: s.Name, File: "SKILL.md", Message: "a leading-slash token (a slash command or absolute path) is not portable across hosts"}}
	case parentTraverse.MatchString(stripped):
		return []Finding{{Rule: RuleNonPortableToken, Skill: s.Name, File: "SKILL.md", Message: "\"../\" (a path outside the skill's own directory) is not portable across hosts"}}
	}
	return nil
}
