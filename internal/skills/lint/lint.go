// Package lint is the executable form of ai-flow/docs/skill-authoring.md:
// every rule that document states is one function in this package,
// returning Findings tagged with the rule id skill-authoring.md itself
// names (or, for the handful of rules that document names without an id
// of their own, an id this package assigns — see the plan's design
// decision 1).
//
// Every rule is an explicit Go check against a skills.Skill's already-
// parsed frontmatter map and body string, never a call into
// internal/schema.Validate at runtime — see the plan's design decision 2
// for why: this package's whole job is attributing a failure to exactly
// one of nineteen ids, and a third-party JSON Schema library's free-text
// error messages are not a contract this codebase controls. The schema
// itself (schema/skill-frontmatter.schema.json) is still tightened in
// the same phase and is exercised by internal/schema's own tests, as the
// one machine-readable statement of the profile's shape for any other
// caller.
package lint

import (
	"bytes"
	"cmp"
	"slices"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// Finding is one lint violation.
type Finding struct {
	// Rule is one of the Rule* constants.
	Rule string
	// Skill is the skill directory name the finding belongs to, or ""
	// for a finding that is not scoped to one skill (none of this
	// phase's rules produce one today; kept for a future cross-repo
	// rule to use without a Result shape change).
	Skill string
	// File is the file the finding points at, relative to the skill's
	// own directory (e.g. "SKILL.md", "references/tdd.md").
	File string
	// Message is the one-line, human-readable reason.
	Message string
}

// Result is Lint's return value.
type Result struct {
	// Root is the directory Lint was called with.
	Root string
	// Skills is every skill name Lint examined, sorted.
	Skills []string
	// Findings is every hard violation, sorted by (Rule, Skill, File,
	// Message) for byte-stable output across runs.
	Findings []Finding
	// Warnings is every soft, non-failing notice (today: only
	// RuleBodyTooLong's soft threshold) — plain strings, since nothing
	// downstream needs to key off a warning's rule id the way a
	// Finding's caller does.
	Warnings []string
}

// Lint runs every rule against root/skills/*/SKILL.md. root not having a
// skills/ directory at all is not an error — Result.Skills is simply
// empty, matching skills.DiscoverSkills's own tolerance (a plugin
// repository between phase 1 and its first skill, or a fixture root
// deliberately narrowed to one skill, are both legitimate empty or
// near-empty inputs).
func Lint(root string) (Result, error) {
	discovered, err := skills.DiscoverSkills(root)
	if err != nil {
		return Result{}, err
	}

	res := Result{Root: root}
	for _, s := range discovered {
		res.Skills = append(res.Skills, s.Name)
	}

	terms := loadTerminology(root)
	refsBySkill := make(map[string]map[string][]byte, len(discovered))

	for _, s := range discovered {
		refs := readReferenceFiles(s.Dir)
		refsBySkill[s.Name] = refs

		res.Findings = append(res.Findings, checkFrontmatter(s)...)
		res.Findings = append(res.Findings, checkBody(s)...)
		res.Findings = append(res.Findings, checkReferences(s, refs)...)
		res.Findings = append(res.Findings, checkDirectory(s)...)
		res.Findings = append(res.Findings, checkAddonSections(s)...)
		res.Findings = append(res.Findings, checkEvals(s)...)
		res.Findings = append(res.Findings, checkTerminology(s, terms)...)

		findings, warnings := checkBodyLength(s)
		res.Findings = append(res.Findings, findings...)
		res.Warnings = append(res.Warnings, warnings...)
	}
	res.Findings = append(res.Findings, checkReferencesIdentical(res.Skills, refsBySkill)...)

	sortFindings(res.Findings)
	slices.Sort(res.Warnings)
	return res, nil
}

// countLines counts data's lines the way a text editor would: a
// trailing newline does not start a new, empty line, but content with
// no trailing newline still counts as one more line. Shared by
// checkBodyLength (directory.go) and checkReferences (references.go).
func countLines(data []byte) int {
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		n++
	}
	return n
}

func sortFindings(findings []Finding) {
	slices.SortFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.Rule, b.Rule),
			cmp.Compare(a.Skill, b.Skill),
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Message, b.Message),
		)
	})
}
