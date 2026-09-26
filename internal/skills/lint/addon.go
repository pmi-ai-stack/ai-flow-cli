package lint

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// addonSections is the five fixed headings every addon-<tool> skill must
// carry, in the order the spec's own table lists them. Presence is
// checked by name-pattern (rule 18, addon-section-missing) regardless of
// whether a real addon of that name exists; content-vs-capability
// consistency (rule 19, addon-section-mismatch, phase 29) is checked
// only for a skill name matching a real ops.Addon — see
// checkAddonSectionCapabilities and .ai/phase-10/plan.md's design
// decision 9 for why the two were split across phases in the first
// place (internal/ops.Addon and the real addons did not exist in
// phase 10).
var addonSections = []string{"Drift", "Changeset", "Impact", "Rollback", "Testing"}

// addonSectionPatterns compiles each of addonSections' five fixed
// headings once, keyed by section name, instead of recompiling the same
// regexp on every addon skill checkAddonSections examines.
var addonSectionPatterns = sync.OnceValue(func() map[string]*regexp.Regexp {
	m := make(map[string]*regexp.Regexp, len(addonSections))
	for _, section := range addonSections {
		m[section] = regexp.MustCompile(`(?m)^##\s+` + regexp.QuoteMeta(section) + `\s*$`)
	}
	return m
})

// checkAddonSections runs rule 18 (addon-section-missing) and, for a
// skill name matching a real ops.Addon, rule 19 (addon-section-mismatch).
func checkAddonSections(s skills.Skill) []Finding {
	if !isAddonSkill(s.Name) {
		return nil
	}
	patterns := addonSectionPatterns()
	var missing []string
	sectionBody := make(map[string]string, len(addonSections))
	for _, section := range addonSections {
		loc := patterns[section].FindStringIndex(s.Body)
		if loc == nil {
			missing = append(missing, section)
			continue
		}
		sectionBody[section] = bodyAfterHeading(s.Body, loc[1])
	}
	if len(missing) > 0 {
		return []Finding{mdFinding(RuleAddonSectionMissing, s,
			fmt.Sprintf("missing required section(s): %s", strings.Join(missing, ", ")))}
	}
	return checkAddonSectionCapabilities(s, sectionBody)
}

// bodyAfterHeading returns body's content between a section's own
// heading (body[start:], start already past the "## Section" line) and
// the next "## " heading or end of body — the section's own content,
// for isMarkedNotApplicable to examine. A plain substring scan, the
// same style internal/cli/ops_rollback.go's own nonRevertibleSection
// already uses for the identical "next ## heading or EOF" boundary.
func bodyAfterHeading(body string, start int) string {
	rest := body[start:]
	if idx := strings.Index(rest, "\n## "); idx >= 0 {
		rest = rest[:idx]
	}
	return rest
}

// checkAddonSectionCapabilities runs rule 19: addon-section-mismatch —
// a section's real-vs-not_applicable content disagrees with what this
// tool's real ops.Addon.Capabilities() says is required. A skill name
// with no matching real addon (e.g. addon-example-tool, this package's
// own structural fixture since phase 10) is skipped entirely: this
// check only applies where a real capability truth exists to check
// against.
func checkAddonSectionCapabilities(s skills.Skill, sectionBody map[string]string) []Finding {
	tool := strings.TrimPrefix(s.Name, "addon-")
	addon, ok := addonRegistry().Lookup(tool)
	if !ok {
		return nil
	}
	caps := addon.Capabilities()
	var findings []Finding
	for _, section := range addonSections {
		required := sectionRequired(caps, section)
		notApplicable := isMarkedNotApplicable(sectionBody[section])
		switch {
		case required && notApplicable:
			findings = append(findings, mdFinding(RuleAddonSectionMismatch, s,
				fmt.Sprintf("%s: %s declares this capability; the section must not be not_applicable", section, tool)))
		case !required && !notApplicable:
			findings = append(findings, mdFinding(RuleAddonSectionMismatch, s,
				fmt.Sprintf("%s: %s does not declare this capability; the section must be the one-line not_applicable form", section, tool)))
		}
	}
	return findings
}
