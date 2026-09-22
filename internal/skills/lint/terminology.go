package lint

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// terminologyFile mirrors ai-flow/docs/terminology.json's shape.
type terminologyFile struct {
	Terms []struct {
		Term  string   `json:"term"`
		Avoid []string `json:"avoid"`
	} `json:"terms"`
}

// avoidTerm is one (avoid-word, preferred-term) pair, pre-compiled to a
// whole-word, case-insensitive pattern.
type avoidTerm struct {
	pattern   *regexp.Regexp
	avoid     string
	preferred string
}

// loadTerminology reads root/docs/terminology.json. Its absence is not
// an error — a lint root with no terminology file (every one of this
// package's own single-rule fixtures except the terminology one) simply
// runs rule 17 as a no-op, exactly like skills.DiscoverSkills tolerates
// a root with no skills/ directory at all.
func loadTerminology(root string) []avoidTerm {
	path := filepath.Join(root, "docs", "terminology.json")
	data, err := handoff.ReadArtifact(path)
	if err != nil || data == nil {
		return nil
	}

	var tf terminologyFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil
	}

	var terms []avoidTerm
	for _, t := range tf.Terms {
		for _, a := range t.Avoid {
			terms = append(terms, avoidTerm{
				pattern:   regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(a) + `\b`),
				avoid:     a,
				preferred: t.Term,
			})
		}
	}
	return terms
}

// headingLinePattern matches a Markdown heading line, stripped before
// the terminology scan runs — see checkTerminology's own doc comment
// for why: the SKILL.md template mandates fixed section names
// ("## Workflow" among them) that this rule must not treat as prose
// choosing the wrong word.
var headingLinePattern = regexp.MustCompile(`(?m)^#+\s.*$`)

// checkTerminology runs rule 17 against the body with every recognized
// code span and every heading line blanked out first, so neither a
// legitimate identifier/command argument nor one of the template's own
// mandated section headings (§ SKILL.md template's fixed "## Workflow"
// among them — a structural label, not a prose word choice) is ever
// flagged.
func checkTerminology(s skills.Skill, terms []avoidTerm) []Finding {
	if len(terms) == 0 {
		return nil
	}
	prose := codeSpanPattern.ReplaceAllString(s.Body, "")
	prose = headingLinePattern.ReplaceAllString(prose, "")

	var findings []Finding
	for _, t := range terms {
		if t.pattern.MatchString(prose) {
			findings = append(findings, mdFinding(RuleTerminology, s,
				fmt.Sprintf("uses %q where %q was meant", t.avoid, t.preferred)))
		}
	}
	return findings
}
