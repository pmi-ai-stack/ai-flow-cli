package lint

import (
	"fmt"
	"os"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// checkDirectory runs rule 14: a skill directory may contain only
// SKILL.md, references/ and evals/.
func checkDirectory(s skills.Skill) []Finding {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil
	}
	var findings []Finding
	for _, e := range entries {
		name := e.Name()
		if name == "SKILL.md" || name == "references" || name == "evals" {
			continue
		}
		findings = append(findings, Finding{
			Rule: RuleDirectoryNotAllowed, Skill: s.Name, File: name,
			Message: fmt.Sprintf("%q is not one of SKILL.md, references/, evals/", name),
		})
	}
	return findings
}

// checkBodyLength runs rule 15: a body over bodyHardLimit lines is a
// Finding; a body over its soft threshold (bodyWarnEntry for develop/
// ship, bodyWarnStage for every other skill) is an envelope warning, not
// a Finding — the spec's own wording ("a warning above 150/250 lines")
// is a notice, distinct from the 500-line hard failure.
func checkBodyLength(s skills.Skill) (findings []Finding, warnings []string) {
	lines := countLines([]byte(s.Body))

	if lines > bodyHardLimit {
		findings = append(findings, mdFinding(RuleBodyTooLong, s,
			fmt.Sprintf("body is %d lines, over the %d-line hard limit", lines, bodyHardLimit)))
		return findings, nil
	}

	soft := bodyWarnStage
	if isEntryPoint(s.Name) {
		soft = bodyWarnEntry
	}
	if lines > soft {
		warnings = append(warnings, fmt.Sprintf("%s: body is %d lines, over the %d-line soft limit", s.Name, lines, soft))
	}
	return findings, warnings
}
