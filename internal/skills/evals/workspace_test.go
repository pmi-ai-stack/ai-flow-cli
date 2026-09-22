package evals

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewWorkspace(t *testing.T) {
	root := t.TempDir()
	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	if w.Key != "PROJ-1" {
		t.Errorf("Key = %q, want PROJ-1", w.Key)
	}
	if _, err := os.Stat(filepath.Join(w.AIFlowHome(), "config.json")); err != nil {
		t.Errorf("config.json was not written under AIFlowHome(): %v", err)
	}
	wantAIFlowHome := filepath.Join(w.HomeDir, ".ai-flow")
	if w.AIFlowHome() != wantAIFlowHome {
		t.Errorf("AIFlowHome() = %q, want %q (must match internal/cli.defaultHome()'s own join)", w.AIFlowHome(), wantAIFlowHome)
	}
}

func TestWorkspace_StageFixture(t *testing.T) {
	root := t.TempDir()
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "status.json"), []byte(`{"stages":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	if err := w.StageFixture(fixture); err != nil {
		t.Fatalf("StageFixture: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(w.TicketDir(), "status.json"))
	if err != nil {
		t.Fatalf("staged status.json missing: %v", err)
	}
	if string(got) != `{"stages":[]}` {
		t.Errorf("staged status.json = %q", got)
	}
}

func TestWorkspace_StageFixture_RefusesExistingTicketDir(t *testing.T) {
	root := t.TempDir()
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "status.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	if err := os.MkdirAll(w.TicketDir(), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := w.StageFixture(fixture); err == nil {
		t.Fatal("expected StageFixture to refuse an already-existing ticket directory")
	}
}

func TestWorkspace_StageSkills(t *testing.T) {
	pluginRoot := t.TempDir()
	skillDir := filepath.Join(pluginRoot, "skills", "hello")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: hello\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	if err := w.StageSkills(pluginRoot); err != nil {
		t.Fatalf("StageSkills: %v", err)
	}

	if _, err := os.Stat(filepath.Join(w.ProjectDir, ".claude", "skills", "hello", "SKILL.md")); err != nil {
		t.Errorf("hello/SKILL.md was not installed into .claude/skills: %v", err)
	}
}
