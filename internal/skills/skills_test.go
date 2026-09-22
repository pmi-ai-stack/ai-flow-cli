package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDiscoverSkills_NoSkillsDir(t *testing.T) {
	root := t.TempDir()
	got, err := DiscoverSkills(root)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DiscoverSkills(no skills/) = %v, want empty", got)
	}
}

func TestDiscoverSkills_SortedAndParsed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "zeta", "SKILL.md"), "---\nname: zeta\ndescription: Z.\n---\n\nBody.\n")
	writeFile(t, filepath.Join(root, "skills", "alpha", "SKILL.md"), "---\nname: alpha\ndescription: A.\n---\n\nBody.\n")
	// A directory without SKILL.md must be ignored, not error.
	if err := os.MkdirAll(filepath.Join(root, "skills", "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverSkills(root)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("DiscoverSkills: got %d skills, want 2 (%+v)", len(got), got)
	}
	if got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Errorf("DiscoverSkills order = [%s, %s], want [alpha, zeta]", got[0].Name, got[1].Name)
	}
	if got[0].Frontmatter["name"] != "alpha" {
		t.Errorf("Frontmatter[name] = %v, want alpha", got[0].Frontmatter["name"])
	}
	if got[0].Body != "Body.\n" {
		t.Errorf("Body = %q, want %q", got[0].Body, "Body.\n")
	}
	if !got[0].FrontmatterOpensFirstLine {
		t.Errorf("FrontmatterOpensFirstLine = false, want true")
	}
}

func TestParseSkill_LeadingBlankLine(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "broken", "SKILL.md"), "\n---\nname: broken\ndescription: d\n---\nBody.\n")

	got, err := DiscoverSkills(root)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("DiscoverSkills: got %d, want 1", len(got))
	}
	if got[0].FrontmatterOpensFirstLine {
		t.Errorf("FrontmatterOpensFirstLine = true, want false (leading blank line)")
	}
	// The frontmatter block itself still parses fine despite the leading
	// blank line — this is what lets lint's frontmatter-not-first-line
	// rule fire alone, without every other frontmatter rule cascading
	// from a nil map.
	if got[0].Frontmatter["name"] != "broken" {
		t.Errorf("Frontmatter[name] = %v, want \"broken\" (parseable despite the leading blank line)", got[0].Frontmatter["name"])
	}
	if got[0].Body != "Body.\n" {
		t.Errorf("Body = %q, want %q", got[0].Body, "Body.\n")
	}
}

func TestParseSkill_ProseBeforeFence(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "broken", "SKILL.md"), "Some prose.\n---\nname: broken\n---\nBody.\n")

	got, err := DiscoverSkills(root)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if got[0].FrontmatterOpensFirstLine {
		t.Errorf("FrontmatterOpensFirstLine = true, want false (prose before the fence)")
	}
	if got[0].Frontmatter != nil {
		t.Errorf("Frontmatter = %+v, want nil (no fence recognized at all)", got[0].Frontmatter)
	}
}

func TestParseSkill_NoClosingFence(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "broken", "SKILL.md"), "---\nname: broken\ndescription: no closing fence\n")

	got, err := DiscoverSkills(root)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	// The opening fence is still literally line 0 — the position rule
	// is satisfied. It is the missing closing fence (Frontmatter == nil)
	// that is the actual defect here, a separate concern.
	if !got[0].FrontmatterOpensFirstLine {
		t.Errorf("FrontmatterOpensFirstLine = false, want true (the opening fence is at line 0)")
	}
	if got[0].Frontmatter != nil {
		t.Errorf("Frontmatter = %+v, want nil", got[0].Frontmatter)
	}
}

func TestParseSkill_MalformedYAML(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "broken", "SKILL.md"), "---\nname: [unclosed\n---\nBody.\n")

	got, err := DiscoverSkills(root)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if got[0].Frontmatter != nil {
		t.Errorf("Frontmatter = %+v, want nil on malformed YAML", got[0].Frontmatter)
	}
	if !got[0].FrontmatterOpensFirstLine {
		t.Errorf("FrontmatterOpensFirstLine = false, want true (fences were found even though the YAML inside is bad)")
	}
}
