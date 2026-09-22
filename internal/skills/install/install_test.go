package install

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

func TestInstall_CopiesSkills(t *testing.T) {
	from := t.TempDir()
	into := t.TempDir()

	writeFile(t, filepath.Join(from, "skills", "hello", "SKILL.md"), "---\nname: hello\n---\nBody.\n")
	writeFile(t, filepath.Join(from, "skills", "hello", "references", "tdd.md"), "# TDD\n")

	res, err := Install(from, into)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(res.Skills) != 1 || res.Skills[0] != "hello" {
		t.Fatalf("Install Skills = %v, want [hello]", res.Skills)
	}

	got, err := os.ReadFile(filepath.Join(into, "hello", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}
	if string(got) != "---\nname: hello\n---\nBody.\n" {
		t.Errorf("installed SKILL.md = %q, want the source content byte-for-byte", got)
	}
	if _, err := os.ReadFile(filepath.Join(into, "hello", "references", "tdd.md")); err != nil {
		t.Errorf("installed references/tdd.md missing: %v", err)
	}
}

func TestInstall_IdempotentReRunPicksUpChange(t *testing.T) {
	from := t.TempDir()
	into := t.TempDir()
	writeFile(t, filepath.Join(from, "skills", "hello", "SKILL.md"), "v1\n")

	if _, err := Install(from, into); err != nil {
		t.Fatalf("first Install: %v", err)
	}

	writeFile(t, filepath.Join(from, "skills", "hello", "SKILL.md"), "v2\n")
	if _, err := Install(from, into); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(into, "hello", "SKILL.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "v2\n" {
		t.Errorf("SKILL.md = %q after re-install, want %q", got, "v2\n")
	}
}

func TestInstall_DoesNotTouchSiblings(t *testing.T) {
	from := t.TempDir()
	into := t.TempDir()
	writeFile(t, filepath.Join(from, "skills", "hello", "SKILL.md"), "hello\n")
	writeFile(t, filepath.Join(into, "someone-elses-skill", "SKILL.md"), "not ours\n")

	if _, err := Install(from, into); err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(into, "someone-elses-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("sibling skill was removed or unreadable: %v", err)
	}
	if string(got) != "not ours\n" {
		t.Errorf("sibling skill content changed: %q", got)
	}
}

func TestInstall_NoSkillsDirIsEmptyNotError(t *testing.T) {
	from := t.TempDir()
	into := t.TempDir()

	res, err := Install(from, into)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(res.Skills) != 0 {
		t.Errorf("Skills = %v, want empty", res.Skills)
	}
}
