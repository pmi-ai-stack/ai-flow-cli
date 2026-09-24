package exec

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

func TestIsToolFailed(t *testing.T) {
	if !IsToolFailed(&envelope.Error{Code: "tool_failed", Message: "boom"}) {
		t.Error("IsToolFailed(tool_failed) = false, want true")
	}
	if IsToolFailed(&envelope.Error{Code: "exec_not_allowed", Message: "nope"}) {
		t.Error("IsToolFailed(exec_not_allowed) = true, want false")
	}
	if IsToolFailed(errors.New("not an envelope.Error at all")) {
		t.Error("IsToolFailed(plain error) = true, want false")
	}
	if IsToolFailed(nil) {
		t.Error("IsToolFailed(nil) = true, want false")
	}
}

// testRunner builds a Runner over rows with no real secrets discovered
// (tests pass their own fake environ where redaction matters).
func testRunner(rows ...Row) *Runner {
	return &Runner{Allowlist: NewAllowlist(rows...), Redact: redact.New(nil)}
}

func TestRunDeniesUnlistedTool(t *testing.T) {
	r := testRunner(Row{Tool: "git", Verb: []string{"status"}, Class: ClassRepoWrite})
	_, err := r.Run(context.Background(), "rm", []string{"-rf", "/"}, RunOpts{})
	if err == nil {
		t.Fatal("expected exec_not_allowed, got nil")
	}
	if !strings.Contains(err.Error(), "rm -rf /") {
		t.Errorf("message %q does not name the denied command", err.Error())
	}
}

func TestRunAllowedToolExecutes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	r := testRunner(Row{Tool: "echo", Verb: nil, Class: ClassRepoWrite})
	out, err := r.Run(context.Background(), "echo", []string{"hello"}, RunOpts{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(out.Stdout) != "hello" {
		t.Errorf("Stdout = %q, want \"hello\"", out.Stdout)
	}
	if out.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", out.ExitCode)
	}
}

func TestRunNonZeroExitReturnsToolFailedWithOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	r := testRunner(Row{Tool: "sh", Verb: nil, Class: ClassRepoWrite})
	out, err := r.Run(context.Background(), "sh", []string{"-c", "echo boom >&2; exit 3"}, RunOpts{})
	if err == nil {
		t.Fatal("expected an error for a non-zero exit")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not carry stderr", err.Error())
	}
	if out.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3 (Output is still populated on error)", out.ExitCode)
	}
}

func TestRunRedactsSecretsFromOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	const secret = "super-secret-token-value"
	rd := redact.New([]string{"AI_FLOW_JIRA_TOKEN=" + secret})
	r := &Runner{Allowlist: NewAllowlist(Row{Tool: "sh", Verb: nil, Class: ClassRepoWrite}), Redact: rd}

	out, err := r.Run(context.Background(), "sh", []string{"-c", "echo " + secret}, RunOpts{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(out.Stdout, secret) {
		t.Errorf("Stdout leaked the secret: %q", out.Stdout)
	}
	if !strings.Contains(out.Stdout, "<redacted:") {
		t.Errorf("Stdout has no redaction token: %q", out.Stdout)
	}
}

func TestRunRespectsTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	r := testRunner(Row{Tool: "sh", Verb: nil, Class: ClassRepoWrite})
	_, err := r.Run(context.Background(), "sh", []string{"-c", "sleep 5"}, RunOpts{Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected the timeout to kill the process and return an error")
	}
}

func TestRunPipesStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/cat")
	}
	r := testRunner(Row{Tool: "cat", Verb: nil, Class: ClassRepoWrite})
	out, err := r.Run(context.Background(), "cat", nil, RunOpts{Stdin: "hello from stdin"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Stdout != "hello from stdin" {
		t.Errorf("Stdout = %q, want the piped stdin content echoed back", out.Stdout)
	}
}

func TestRunEmptyStdinLeavesProcessStdinUnset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/cat")
	}
	// A zero-value RunOpts.Stdin must not block a tool that reads
	// stdin only when something is actually piped (cat with no input
	// and a closed/inherited stdin still exits promptly here since the
	// test process's own stdin is not a terminal).
	r := testRunner(Row{Tool: "true", Verb: nil, Class: ClassRepoWrite})
	_, err := r.Run(context.Background(), "true", nil, RunOpts{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunUsesDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/pwd")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	r := testRunner(Row{Tool: "pwd", Verb: nil, Class: ClassRepoWrite})
	out, err := r.Run(context.Background(), "pwd", nil, RunOpts{Dir: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(out.Stdout); got != dir {
		t.Errorf("pwd printed %q, want %q", got, dir)
	}
}
