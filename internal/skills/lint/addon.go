package lint

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// addonSections is the five fixed headings every addon-<tool> skill must
// carry, in the order the spec's own table lists them. This phase
// checks presence only, by name-pattern — see the plan's design
// decision 9 for why capability-aware validation waits for
// internal/ops.Addon (phase 18) and the real addons (phase 21+).
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

// checkAddonSections runs rule 18: addon-section-missing.
func checkAddonSections(s skills.Skill) []Finding {
	if !isAddonSkill(s.Name) {
		return nil
	}
	patterns := addonSectionPatterns()
	var missing []string
	for _, section := range addonSections {
		if !patterns[section].MatchString(s.Body) {
			missing = append(missing, section)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []Finding{mdFinding(RuleAddonSectionMissing, s,
		fmt.Sprintf("missing required section(s): %s", strings.Join(missing, ", ")))}
}
