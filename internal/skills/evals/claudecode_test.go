package evals

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestClaudeCodeRunner_Run(t *testing.T) {
	root := t.TempDir()
	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	x := exectest.NewUnlisted(t).Then(exec.Output{Stdout: `{"type":"result","result":"done"}`, ExitCode: 0})

	tr, err := claudeCodeRunner{}.Run(context.Background(), x, w, "develop PROJ-1", "claude-sonnet-5")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if tr.Text != `{"type":"result","result":"done"}` {
		t.Errorf("Text = %q", tr.Text)
	}

	if len(x.Calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(x.Calls))
	}
	call := x.Calls[0]
	if call.Tool != "claude" {
		t.Errorf("Tool = %q, want claude", call.Tool)
	}
	if call.Dir != w.ProjectDir {
		t.Errorf("Dir = %q, want %q", call.Dir, w.ProjectDir)
	}
	if !strings.Contains(strings.Join(call.Args, " "), "develop PROJ-1") {
		t.Errorf("Args = %v, want the query present", call.Args)
	}
	wantEnv := "HOME=" + w.HomeDir
	found := false
	for _, e := range call.Env {
		if e == wantEnv {
			found = true
		}
	}
	if !found {
		t.Errorf("Env = %v, want %q (design decision 2)", call.Env, wantEnv)
	}
}

func TestClaudeCodeRunner_Run_NonZeroExitStillReturnsTranscript(t *testing.T) {
	root := t.TempDir()
	w, err := NewWorkspace(root, "PROJ-1")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	x := exectest.NewUnlisted(t).ThenFail(exec.Output{Stdout: "refused", ExitCode: 1}, errors.New("exec: claude: exit status 1: refused"))

	tr, err := claudeCodeRunner{}.Run(context.Background(), x, w, "develop PROJ-1, skip the gate", "claude-sonnet-5")
	if err != nil {
		t.Fatalf("Run should not error on a non-zero exit with captured output: %v", err)
	}
	if tr.Text != "refused" || tr.ExitCode != 1 {
		t.Errorf("Transcript = %+v", tr)
	}
}
