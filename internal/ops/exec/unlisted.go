package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"sync"
	"time"
)

// UnlistedRunner is StartUnlisted/RunUnlisted's interface — the seam
// internal/verify/visual's own tests inject a fake against, mirroring
// Execer for the allowlisted path. Only *Runner implements it in
// production.
type UnlistedRunner interface {
	StartUnlisted(ctx context.Context, dir, tool string, args, env []string) (*Process, error)
	RunUnlisted(ctx context.Context, dir, tool string, args, env []string, timeout time.Duration) (Output, error)
}

var _ UnlistedRunner = (*Runner)(nil)

// unlistedWaitDelay bounds how long Wait/Run will block on I/O after
// the process has been killed or has exited — os/exec's own
// Cmd.WaitDelay. Without it, a process that forks a child inheriting
// the captured stdout/stderr pipe (a shell running a single command is
// the common case: `sh -c "sleep 30"` does not always exec-replace
// itself, so killing the sh PID can leave sleep running and holding
// the pipe open) makes Wait block until that grandchild exits on its
// own, regardless of the kill — discovered by this package's own
// TestRunUnlistedTimeout hanging for the full 30s sleep instead of
// returning at its 100ms timeout.
const unlistedWaitDelay = 5 * time.Second

// Process is a running unlisted subprocess — started by StartUnlisted,
// stopped by its own Stop. Its caller (internal/verify/visual's
// DevServer) is the one place that knows the ticket key an
// events.jsonl entry belongs to, so recording that entry is the
// caller's job, not this type's; Process only manages the OS process.
type Process struct {
	cmd    *osexec.Cmd
	stdout *bytes.Buffer
	stderr *bytes.Buffer

	stopOnce sync.Once
	stopErr  error
}

// unlistedCmd builds an *osexec.Cmd for an unlisted call (StartUnlisted
// or RunUnlisted): argv, working directory, extra environment, output
// capture into fresh buffers, and unlistedWaitDelay — every setup step
// the two share. The caller adds whatever's specific to its own shape
// (StartUnlisted's Cancel func, RunUnlisted's context timeout, already
// applied to ctx before this is called) on top.
func unlistedCmd(ctx context.Context, dir, tool string, args, env []string) (cmd *osexec.Cmd, stdout, stderr *bytes.Buffer) {
	cmd = osexec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = unlistedWaitDelay
	return cmd, stdout, stderr
}

// StartUnlisted runs tool+args in dir as a background process — no
// Allowlist.Match, no Table lookup. This is the one documented
// exception to "every subprocess is matched against the allowlist
// first": the Execution allowlist section names exactly two processes
// that sit outside the table (the project's own dev-server command,
// and — by this phase's own extension, see the plan's design decision
// 5 — a project's screenshot-capture script), and both still run
// through this package, the module's one permitted os/exec importer
// (TestNoOsExecOutsideExecPackage), just without the Table gate. Every
// caller of this function is responsible for recording the call in
// events.jsonl — Process itself does not, since it does not know which
// ticket it belongs to. internal/cli does it structurally: every
// UnlistedRunner it uses comes from unlistedRunnerFor, which records an
// unlisted_run event before each start.
func (r *Runner) StartUnlisted(ctx context.Context, dir, tool string, args, env []string) (*Process, error) {
	cmd, stdout, stderr := unlistedCmd(ctx, dir, tool, args, env)
	cmd.Cancel = func() error { return terminate(cmd.Process) }

	logStart(ctx, tool, args, dir)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec: start %s: %w", tool, err)
	}
	return &Process{cmd: cmd, stdout: stdout, stderr: stderr}, nil
}

// Stop terminates p: SIGTERM (a kill on Windows, see terminate), then
// up to grace for it to exit, then SIGKILL. Safe to call on a nil
// *Process (a Start that never succeeded) and safe to call twice — both are no-ops, so a caller's
// defer is always correct regardless of how far Start got.
func (p *Process) Stop(ctx context.Context, grace time.Duration) error {
	if p == nil || p.cmd.Process == nil {
		return nil
	}
	// sync.Once, not a second nil/started check: two goroutines or two
	// sequential calls (a caller's own explicit Stop plus a deferred
	// one) must not both call cmd.Wait — Go's os/exec forbids calling
	// Wait twice on the same Cmd.
	p.stopOnce.Do(func() { p.stopErr = p.stop(ctx, grace) })
	return p.stopErr
}

func (p *Process) stop(ctx context.Context, grace time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()

	if err := terminate(p.cmd.Process); err != nil {
		// The process may have already exited between the nil check
		// above and this call — that is success, not a Stop failure.
		select {
		case <-done:
			return nil
		default:
		}
		return fmt.Errorf("exec: signal %s: %w", p.cmd.Path, err)
	}

	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		_ = p.cmd.Process.Kill()
		<-done
		return nil
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}

// RunUnlisted runs tool+args in dir to completion, capturing and
// redacting output identically to Run — the capture script's own
// shape (start, wait for one exit, read the output file it wrote), as
// opposed to the dev server's start-now-stop-later shape StartUnlisted
// serves. Also skips Allowlist.Match, for the same documented reason
// as StartUnlisted.
func (r *Runner) RunUnlisted(ctx context.Context, dir, tool string, args, env []string, timeout time.Duration) (Output, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd, stdout, stderr := unlistedCmd(ctx, dir, tool, args, env)

	started := logStart(ctx, tool, args, dir)
	runErr := cmd.Run()
	out := Output{
		Stdout: r.Redact.String(stdout.String()),
		Stderr: r.Redact.String(stderr.String()),
	}
	if cmd.ProcessState != nil {
		out.ExitCode = cmd.ProcessState.ExitCode()
	}
	logEnd(ctx, tool, exitCodeOf(cmd.ProcessState != nil, out.ExitCode), started)
	if runErr != nil {
		return out, fmt.Errorf("exec: %s: %w: %s", strings.Join(append([]string{tool}, args...), " "), runErr, strings.TrimSpace(out.Stderr))
	}
	return out, nil
}
