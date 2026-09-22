package gitops

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestPrepareRefusesDirtyWorktree(t *testing.T) {
	fe := exectest.New(t).On("git", []string{"status", "--porcelain"}, exec.Output{Stdout: " M some/file.go\n"})
	g := New(fe, 0)

	_, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: "/repo", Base: "main", Branch: "feature/PROJ-1", Home: "/home", Key: "PROJ-1", RepoName: "acme.api",
	})
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "dirty_worktree" {
		t.Fatalf("err = %v, want dirty_worktree", err)
	}
	if fe.CalledWithPrefix("git", "fetch") {
		t.Error("fetch should never run once the dirty check fails")
	}
}

func TestPrepareRefusesDivergedBase(t *testing.T) {
	fe := exectest.New(t).
		On("git", []string{"status", "--porcelain"}, exec.Output{}).
		On("git", []string{"fetch", "origin", "main"}, exec.Output{}).
		On("git", []string{"rev-parse", "--verify", "main"}, exec.Output{ExitCode: 0}).
		On("git", []string{"rev-list", "--count", "origin/main..main"}, exec.Output{Stdout: "2\n"})
	g := New(fe, 0)

	_, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: "/repo", Base: "main", Branch: "feature/PROJ-1", Home: "/home", Key: "PROJ-1", RepoName: "acme.api",
	})
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "base_diverged" {
		t.Fatalf("err = %v, want base_diverged", err)
	}
	if fe.CalledWithPrefix("git", "checkout", "-b") || fe.CalledWithPrefix("git", "worktree", "add", "-b") {
		t.Error("branch creation should never run once divergence is detected")
	}
}

func TestPrepareSkipsDivergenceCheckWhenLocalBaseRefDoesNotExist(t *testing.T) {
	home := t.TempDir()
	fe := exectest.New(t).
		On("git", []string{"status", "--porcelain"}, exec.Output{}).
		On("git", []string{"fetch", "origin", "main"}, exec.Output{}).
		OnFail("git", []string{"rev-parse", "--verify", "main"}, errFail).
		OnFail("git", []string{"rev-parse", "--verify", "feature/PROJ-1"}, errFail).
		On("git", []string{"checkout", "-b", "feature/PROJ-1", "origin/main"}, exec.Output{}).
		On("git", []string{"merge-base", "origin/main", "feature/PROJ-1"}, exec.Output{Stdout: "deadbeef\n"}).
		On("git", []string{"worktree", "add", "--detach", BaseWorktreePath(home, "PROJ-1", "acme.api"), "deadbeef"}, exec.Output{})
	g := New(fe, 0)

	res, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: "/repo", Base: "main", Branch: "feature/PROJ-1", Home: home, Key: "PROJ-1", RepoName: "acme.api",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.WorkingPath != "/repo" {
		t.Errorf("WorkingPath = %q, want /repo (plain checkout, no worktree)", res.WorkingPath)
	}
	if !res.BranchCreated {
		t.Error("BranchCreated = false, want true (branch did not exist yet)")
	}
	if fe.CalledWithPrefix("git", "rev-list") {
		t.Error("rev-list should never run when the local base ref does not exist")
	}
}

var errFail = errors.New("fake: ref does not exist")

func TestPrepareCreatesWorktreeWhenConfigured(t *testing.T) {
	home := t.TempDir()
	localPath := filepath.Join(t.TempDir(), "acme.api")
	wantWT := filepath.Join(filepath.Dir(localPath), "acme.api-PROJ-1")

	fe := exectest.New(t).
		On("git", []string{"status", "--porcelain"}, exec.Output{}).
		On("git", []string{"fetch", "origin", "main"}, exec.Output{}).
		OnFail("git", []string{"rev-parse", "--verify", "main"}, errFail).
		On("git", []string{"worktree", "add", "-b", "feature/PROJ-1", wantWT, "origin/main"}, exec.Output{}).
		On("git", []string{"merge-base", "origin/main", "feature/PROJ-1"}, exec.Output{Stdout: "cafebabe\n"}).
		On("git", []string{"worktree", "add", "--detach", BaseWorktreePath(home, "PROJ-1", "acme.api"), "cafebabe"}, exec.Output{})
	g := New(fe, 0)

	res, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: localPath, Base: "main", Branch: "feature/PROJ-1", Worktree: true,
		Home: home, Key: "PROJ-1", RepoName: "acme.api",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.WorkingPath != wantWT {
		t.Errorf("WorkingPath = %q, want %q", res.WorkingPath, wantWT)
	}
	if res.BaseWorktreePath != BaseWorktreePath(home, "PROJ-1", "acme.api") {
		t.Errorf("BaseWorktreePath = %q", res.BaseWorktreePath)
	}
}

func TestPrepareIsIdempotentOnBranchAlreadyPresent(t *testing.T) {
	home := t.TempDir()
	fe := exectest.New(t).
		On("git", []string{"status", "--porcelain"}, exec.Output{}).
		On("git", []string{"fetch", "origin", "main"}, exec.Output{}).
		OnFail("git", []string{"rev-parse", "--verify", "main"}, errFail).
		On("git", []string{"rev-parse", "--verify", "feature/PROJ-1"}, exec.Output{}).
		On("git", []string{"merge-base", "origin/main", "feature/PROJ-1"}, exec.Output{Stdout: "deadbeef\n"}).
		On("git", []string{"worktree", "add", "--detach", BaseWorktreePath(home, "PROJ-1", "acme.api"), "deadbeef"}, exec.Output{})
	g := New(fe, 0)

	res, err := g.Prepare(context.Background(), PrepareInput{
		LocalPath: "/repo", Base: "main", Branch: "feature/PROJ-1", Home: home, Key: "PROJ-1", RepoName: "acme.api",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.BranchCreated {
		t.Error("BranchCreated = true, want false — the branch already existed")
	}
	if fe.CalledWithPrefix("git", "checkout", "-b") {
		t.Error("checkout -b should never run once rev-parse --verify proves the branch already exists")
	}
}
