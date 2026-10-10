package lint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// allowedFrontmatterKeys is the Portable skill profile's complete field
// set — anything else, including allowed-tools, is
// RuleFrontmatterUnknownField.
var allowedFrontmatterKeys = map[string]bool{
	"name":                     true,
	"description":              true,
	"license":                  true,
	"compatibility":            true,
	"disable-model-invocation": true,
}

const fixedCompatibilitySentence = "Requires the ai-flow CLI on PATH and a git checkout of the in-scope repositories."

var namePattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// mdFinding builds a Finding against SKILL.md itself — the one file
// every frontmatter rule in this file points at.
func mdFinding(rule string, s skills.Skill, message string) Finding {
	return Finding{Rule: rule, Skill: s.Name, File: "SKILL.md", Message: message}
}

// checkFrontmatter runs rules 1-6 (frontmatter-not-first-line through
// frontmatter-unknown-field). Each rule reads s.Frontmatter directly —
// see this package's own doc comment for why no call to
// internal/schema.Validate is involved.
func checkFrontmatter(s skills.Skill) []Finding {
	var findings []Finding

	if !s.FrontmatterOpensFirstLine {
		findings = append(findings, mdFinding(RuleFrontmatterNotFirstLine, s,
			"frontmatter must open with \"---\" as the file's literal first line"))
	}

	if s.Frontmatter == nil {
		findings = append(findings, mdFinding(RuleFrontmatterInvalid, s,
			"frontmatter block could not be parsed (missing closing \"---\" fence, or invalid YAML)"))
		return findings
	}

	findings = append(findings, checkUnknownFields(s)...)
	findings = append(findings, checkName(s)...)
	findings = append(findings, checkDescription(s)...)
	findings = append(findings, checkLicenseAndCompatibility(s)...)
	findings = append(findings, checkDisableModelInvocation(s)...)

	return findings
}

func checkUnknownFields(s skills.Skill) []Finding {
	var extra []string
	for k := range s.Frontmatter {
		if !allowedFrontmatterKeys[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	sort.Strings(extra)
	return []Finding{mdFinding(RuleFrontmatterUnknownField, s,
		fmt.Sprintf("unrecognized frontmatter field(s): %s", strings.Join(extra, ", ")))}
}

func checkName(s skills.Skill) []Finding {
	v, ok := s.Frontmatter["name"]
	if !ok {
		return []Finding{mdFinding(RuleNameMatchesDirectory, s, "\"name\" is required")}
	}
	name, ok := v.(string)
	if !ok {
		return []Finding{mdFinding(RuleNameMatchesDirectory, s, "\"name\" must be a string")}
	}
	if !namePattern.MatchString(name) {
		return []Finding{mdFinding(RuleNameMatchesDirectory, s,
			fmt.Sprintf("\"name: %s\" does not match ^[a-z0-9-]{1,64}$", name))}
	}
	if name != s.Name {
		return []Finding{mdFinding(RuleNameMatchesDirectory, s,
			fmt.Sprintf("\"name: %s\" does not equal the directory name %q", name, s.Name))}
	}
	return nil
}

// secondSentenceForms are the fixed forms description's second sentence
// must take, keyed by the skill's own kind. isEntryPoint and isAddonSkill
// are mutually exclusive by construction (an addon skill is never also
// named develop/ship).
func secondSentenceForms(s skills.Skill) (label string, matches func(string) bool) {
	switch {
	case isEntryPoint(s.Name):
		return "\"Use when …\"", func(sentence string) bool {
			return strings.HasPrefix(sentence, "Use when ")
		}
	case isAddonSkill(s.Name):
		return "\"Loaded by OPS stage skills for <tool> targets; not for direct use.\"", func(sentence string) bool {
			return strings.Contains(sentence, "Loaded by OPS stage skills for ") && strings.HasSuffix(strings.TrimSpace(sentence), "not for direct use.")
		}
	default:
		return "\"Loaded by the develop orchestrator at the <stage> stage; not for direct use.\"", func(sentence string) bool {
			return strings.Contains(sentence, "Loaded by the develop orchestrator at the ") && strings.HasSuffix(strings.TrimSpace(sentence), "not for direct use.")
		}
	}
}

func checkDescription(s skills.Skill) []Finding {
	fail := func(msg string) []Finding {
		return []Finding{mdFinding(RuleDescriptionForm, s, msg)}
	}

	v, ok := s.Frontmatter["description"]
	if !ok {
		return fail("\"description\" is required")
	}
	desc, ok := v.(string)
	if !ok {
		return fail("\"description\" must be a string")
	}
	if desc == "" {
		return fail("\"description\" must not be empty")
	}
	if len(desc) > 600 {
		return fail(fmt.Sprintf("\"description\" is %d characters, over the 600 limit", len(desc)))
	}
	for _, bad := range []string{"I ", "We ", "You "} {
		if strings.HasPrefix(desc, bad) {
			return fail("\"description\" must be third person, not first/second person")
		}
	}

	_, after, ok := strings.Cut(desc, ". ")
	if !ok && !strings.HasSuffix(desc, ".") {
		return fail("\"description\" must end with a period")
	}
	if !ok {
		return fail("\"description\" must have at least two sentences (what it does, then Use when … / Loaded by …)")
	}
	second := strings.TrimSpace(after)
	label, matches := secondSentenceForms(s)
	if !matches(second) {
		return fail(fmt.Sprintf("\"description\"'s second sentence must match %s, got %q", label, second))
	}
	return nil
}

// checkConstField appends a Finding when frontmatter's key is missing or
// not exactly want — the shape both license and compatibility share,
// differing only in the field name and its one fixed valid value.
func checkConstField(s skills.Skill, findings *[]Finding, key, want string) {
	v, ok := s.Frontmatter[key]
	switch {
	case !ok:
		*findings = append(*findings, mdFinding(RuleFrontmatterInvalid, s, fmt.Sprintf("%q is required", key)))
	case v != want:
		*findings = append(*findings, mdFinding(RuleFrontmatterInvalid, s, fmt.Sprintf("%q must be exactly %q, got %v", key, want, v)))
	}
}

func checkLicenseAndCompatibility(s skills.Skill) []Finding {
	var findings []Finding
	checkConstField(s, &findings, "license", "MIT")
	checkConstField(s, &findings, "compatibility", fixedCompatibilitySentence)
	return findings
}

func checkDisableModelInvocation(s skills.Skill) []Finding {
	v, present := s.Frontmatter["disable-model-invocation"]

	if !present {
		if isEntryPoint(s.Name) {
			return []Finding{mdFinding(RuleDisableModelInvocationMisplaced, s,
				"entry point skills (develop, ship) must set \"disable-model-invocation: true\"")}
		}
		return nil
	}

	b, ok := v.(bool)
	if !ok {
		return []Finding{mdFinding(RuleFrontmatterInvalid, s, "\"disable-model-invocation\" must be a boolean")}
	}
	if !isEntryPoint(s.Name) {
		return []Finding{mdFinding(RuleDisableModelInvocationMisplaced, s,
			"\"disable-model-invocation\" must be absent on every skill except develop and ship")}
	}
	if !b {
		return []Finding{mdFinding(RuleDisableModelInvocationMisplaced, s,
			"entry point skills (develop, ship) must set \"disable-model-invocation: true\", not false")}
	}
	return nil
}
