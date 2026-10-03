package gitops

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestDiffUsesTripleDotAgainstOrigin(t *testing.T) {
	fe := exectest.New(t).
		On("git", []string{"diff", "--name-only", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: "a.go\nb.go\n"}).
		On("git", []string{"ls-files", "--others", "--exclude-standard"}, exec.Output{Stdout: ""}).
		On("git", []string{"diff", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: "diff --git a/a.go b/a.go\n"})
	g := New(fe, 0)

	res, err := g.Diff(context.Background(), "/repo", "main")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(res.Files) != 2 || res.Files[0] != "a.go" || res.Files[1] != "b.go" {
		t.Errorf("Files = %v, want [a.go b.go]", res.Files)
	}
	if res.Diff == "" {
		t.Error("Diff text is empty")
	}
}

func TestDiffNoChanges(t *testing.T) {
	fe := exectest.New(t).
		On("git", []string{"diff", "--name-only", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: ""}).
		On("git", []string{"ls-files", "--others", "--exclude-standard"}, exec.Output{Stdout: ""}).
		On("git", []string{"diff", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: ""})
	g := New(fe, 0)

	res, err := g.Diff(context.Background(), "/repo", "main")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(res.Files) != 0 {
		t.Errorf("Files = %v, want empty", res.Files)
	}
}

func TestDiffIncludesUntrackedFiles(t *testing.T) {
	fe := exectest.New(t).
		On("git", []string{"diff", "--name-only", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: "a.go\n"}).
		On("git", []string{"ls-files", "--others", "--exclude-standard"}, exec.Output{Stdout: "new-file.txt\n"}).
		On("git", []string{"diff", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: "diff --git a/a.go b/a.go\n"})
	g := New(fe, 0)

	res, err := g.Diff(context.Background(), "/repo", "main")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(res.Files) != 2 || res.Files[0] != "a.go" || res.Files[1] != "new-file.txt" {
		t.Errorf("Files = %v, want [a.go new-file.txt] (tracked + untracked, sorted)", res.Files)
	}
}
