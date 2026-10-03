package gitops

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestCommitSkipsWhenClean(t *testing.T) {
	fe := exectest.New(t).On("git", []string{"status", "--porcelain"}, exec.Output{Stdout: ""})
	g := New(fe, 0)

	committed, err := g.Commit(context.Background(), "/repo", "msg")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed {
		t.Error("committed = true, want false on a clean tree")
	}
	if fe.CalledWithPrefix("git", "add") || fe.CalledWithPrefix("git", "commit") {
		t.Error("add/commit should never run on a clean tree")
	}
}

func TestCommitAddsAndCommitsWhenDirty(t *testing.T) {
	fe := exectest.New(t).
		On("git", []string{"status", "--porcelain"}, exec.Output{Stdout: " M a.go\n"}).
		On("git", []string{"add", "-A"}, exec.Output{}).
		On("git", []string{"commit", "-m", "fix: a thing"}, exec.Output{})
	g := New(fe, 0)

	committed, err := g.Commit(context.Background(), "/repo", "fix: a thing")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Error("committed = false, want true on a dirty tree")
	}
}

func TestIsDirty(t *testing.T) {
	fe := exectest.New(t).On("git", []string{"status", "--porcelain"}, exec.Output{Stdout: " M a.go\n"})
	g := New(fe, 0)
	dirty, err := g.IsDirty(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if !dirty {
		t.Error("IsDirty = false, want true")
	}
}

func TestPushSetsUpstream(t *testing.T) {
	fe := exectest.New(t).On("git", []string{"push", "-u", "--end-of-options", "origin", "feature/PROJ-1"}, exec.Output{})
	g := New(fe, 0)

	if err := g.Push(context.Background(), "/repo", "feature/PROJ-1"); err != nil {
		t.Fatalf("Push: %v", err)
	}
}
