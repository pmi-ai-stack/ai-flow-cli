package lint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// readReferenceFiles reads every non-directory entry under
// dir/references/ once, keyed by filename — the one read Lint shares
// between checkReferences' too-long check and checkReferencesIdentical's
// cross-skill comparison, so a skill's references/ directory is read
// from disk exactly once per Lint call, not twice. A missing
// references/ directory, or a file that fails to read, is tolerated
// with an absent/omitted entry rather than an error, matching this
// package's general tolerance for absent optional content.
func readReferenceFiles(dir string) map[string][]byte {
	entries, err := os.ReadDir(filepath.Join(dir, "references"))
	if err != nil {
		return nil
	}
	refs := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := handoff.ReadArtifact(filepath.Join(dir, "references", e.Name()))
		if err != nil || data == nil {
			continue
		}
		refs[e.Name()] = data
	}
	return refs
}

// checkReferences runs rules 11-12: every references/<file>.md the body
// links to must exist, and every file actually under references/ must
// be at most 80 lines. refs is this skill's own references/ directory,
// already read once by Lint via readReferenceFiles.
func checkReferences(s skills.Skill, refs map[string][]byte) []Finding {
	var findings []Finding

	seen := map[string]bool{}
	for _, m := range referenceLinkPattern.FindAllString(s.Body, -1) {
		if seen[m] {
			continue
		}
		seen[m] = true
		if _, err := os.Stat(filepath.Join(s.Dir, m)); err != nil {
			findings = append(findings, mdFinding(RuleMissingReference, s, fmt.Sprintf("%q is linked but does not exist", m)))
		}
	}

	for name, data := range refs {
		lines := countLines(data)
		if lines > referenceLimit {
			findings = append(findings, Finding{
				Rule: RuleReferenceTooLong, Skill: s.Name, File: "references/" + name,
				Message: fmt.Sprintf("%d lines, over the %d-line limit", lines, referenceLimit),
			})
		}
	}

	return findings
}

// checkReferencesIdentical runs rule 13 across every skill in the lint
// run at once: a references/<file> name carried by more than one skill
// must be byte-identical everywhere it appears. order is every skill's
// name in Lint's own discovery order (the same order Result.Skills
// carries), so which skill is reported as "first" (the comparison
// baseline) is deterministic; refsBySkill is each skill's own
// readReferenceFiles result, already read once by Lint's main loop.
func checkReferencesIdentical(order []string, refsBySkill map[string]map[string][]byte) []Finding {
	type owner struct {
		skill string
		data  []byte
	}
	byName := map[string][]owner{}

	for _, skillName := range order {
		for name, data := range refsBySkill[skillName] {
			byName[name] = append(byName[name], owner{skill: skillName, data: data})
		}
	}

	var findings []Finding
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		owners := byName[name]
		if len(owners) < 2 {
			continue
		}
		first := owners[0]
		var diverged []string
		for _, o := range owners[1:] {
			if !bytes.Equal(o.data, first.data) {
				diverged = append(diverged, o.skill)
			}
		}
		if len(diverged) == 0 {
			continue
		}
		skillNames := make([]string, 0, len(owners))
		for _, o := range owners {
			skillNames = append(skillNames, o.skill)
		}
		findings = append(findings, Finding{
			Rule: RuleReferenceNotIdentical, Skill: first.skill, File: "references/" + name,
			Message: fmt.Sprintf("references/%s differs across skills carrying it: %s", name, strings.Join(skillNames, ", ")),
		})
	}
	return findings
}
