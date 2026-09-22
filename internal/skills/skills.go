// Package skills is the shared "what is a skill" definition for
// internal/skills/lint and internal/skills/install: a directory under
// root/skills/ carrying a SKILL.md, parsed into its YAML frontmatter and
// its Markdown body. Neither child package re-implements discovery or
// parsing — both import this package's DiscoverSkills.
package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Skill is one skills/<name>/ directory: its parsed SKILL.md, ready for
// lint's rule checks or install's copy.
type Skill struct {
	// Name is the directory name under skills/.
	Name string
	// Dir is the skill's absolute directory path.
	Dir string
	// Raw is SKILL.md's complete, unmodified file content.
	Raw []byte
	// Frontmatter is the decoded YAML block between the first two "---"
	// lines, or nil if SKILL.md does not open with one (see
	// FrontmatterOpensFirstLine).
	Frontmatter map[string]any
	// Body is everything after the closing "---" fence, with at most one
	// immediately following blank line trimmed — see the plan's design
	// decision 3 for why this, and not the raw remainder, is "the body"
	// every body-scoped lint rule reads.
	Body string
	// FrontmatterOpensFirstLine is false when the opening "---" fence
	// (found at the file's first non-blank line, if any) is not
	// literally line 0 — a leading blank line before an otherwise
	// well-formed frontmatter block. It says nothing about whether the
	// block closes or parses; Frontmatter being nil is the signal for
	// that (no fence found at all, no closing fence, or invalid YAML).
	FrontmatterOpensFirstLine bool
}

// DiscoverSkills returns every skills/<name>/SKILL.md directly under
// root/skills/, sorted by name. root/skills/ not existing is not an
// error — it returns an empty slice, so a plugin repository with no
// skills yet (or a fixture root testing one rule at a time) is not a
// failure by itself.
func DiscoverSkills(root string) ([]Skill, error) {
	skillsDir := filepath.Join(root, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skills: read %s: %w", skillsDir, err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMD := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillMD); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	result := make([]Skill, 0, len(names))
	for _, name := range names {
		s, err := parseSkill(filepath.Join(skillsDir, name), name)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, nil
}

// frontmatterFence is the "---" line SKILL.md opens and closes its
// frontmatter block with, matched exactly (no trailing spaces) per
// skill-authoring.md's "frontmatter opens on line 1" rule.
const frontmatterFence = "---"

// parseSkill locates the frontmatter block starting at the file's first
// non-blank line (not strictly line 0): a leading blank line still lets
// the frontmatter itself parse normally, so that lint's
// frontmatter-not-first-line rule fires alone, undiluted by every other
// frontmatter rule cascading from a nil map. FrontmatterOpensFirstLine
// records only whether that opening fence was truly line 0 — the
// position rule skill-authoring.md states — independent of whether the
// block inside it parses.
func parseSkill(dir, name string) (Skill, error) {
	path := filepath.Join(dir, "SKILL.md")
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is built from a discovered directory listing, not attacker input
	if err != nil {
		return Skill{}, fmt.Errorf("skills: read %s: %w", path, err)
	}

	s := Skill{Name: name, Dir: dir, Raw: raw}

	lines := bytes.Split(raw, []byte("\n"))
	openAt := -1
	for i, l := range lines {
		trimmed := bytes.TrimSpace(l)
		if len(trimmed) == 0 {
			continue
		}
		if string(bytes.TrimRight(l, "\r")) == frontmatterFence {
			openAt = i
		}
		break
	}
	if openAt == -1 {
		s.Body = string(raw)
		return s, nil
	}
	s.FrontmatterOpensFirstLine = openAt == 0

	closeAt := -1
	for i := openAt + 1; i < len(lines); i++ {
		if string(bytes.TrimRight(lines[i], "\r")) == frontmatterFence {
			closeAt = i
			break
		}
	}
	if closeAt == -1 {
		s.Body = string(raw)
		return s, nil
	}

	fmBlock := bytes.Join(lines[openAt+1:closeAt], []byte("\n"))
	var fm map[string]any
	if err := yaml.Unmarshal(fmBlock, &fm); err == nil {
		s.Frontmatter = fm
	}
	// A YAML parse error leaves Frontmatter nil; checkFrontmatter treats
	// that as one RuleFrontmatterInvalid finding rather than cascading
	// into every field-level rule.

	bodyLines := lines[closeAt+1:]
	if len(bodyLines) > 0 && len(bytes.TrimSpace(bodyLines[0])) == 0 {
		bodyLines = bodyLines[1:]
	}
	s.Body = string(bytes.Join(bodyLines, []byte("\n")))

	return s, nil
}
