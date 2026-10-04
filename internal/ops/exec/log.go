package exec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/runlog"
)

// The logging below is --verbose's subprocess half: every process this
// package starts is reported to the context's runlog.Logger (silent when
// there is none), so `--verbose` shows the commands where most wall-clock
// time and most failures live. Arguments and the message are redacted by
// the logger before they are written.

// logStart reports a process about to start and returns when it did.
func logStart(ctx context.Context, tool string, args []string, dir string) time.Time {
	l := runlog.From(ctx)
	l.Info("exec: "+strings.Join(append([]string{tool}, args...), " "),
		runlog.F("event", "exec_start"), runlog.F("tool", tool), runlog.F("args", args), runlog.F("dir", dir))
	return l.Now()
}

// logDenied reports a command the allowlist refused.
func logDenied(ctx context.Context, tool string, args []string) {
	runlog.From(ctx).Info("exec: refused, not on the allowlist: "+strings.Join(append([]string{tool}, args...), " "),
		runlog.F("event", "exec_denied"), runlog.F("tool", tool), runlog.F("args", args))
}

// logEnd reports a finished process. exitCode is -1 when it never ran
// (the tool was not found, say).
func logEnd(ctx context.Context, tool string, exitCode int, started time.Time) {
	l := runlog.From(ctx)
	d := l.Now().Sub(started)
	l.Info(fmt.Sprintf("exec: %s exited %d in %s", tool, exitCode, d.Round(time.Millisecond)),
		runlog.F("event", "exec_end"), runlog.F("tool", tool), runlog.F("exit_code", exitCode), runlog.F("duration_ms", d.Milliseconds()))
}

// exitCodeOf is cmd's exit code, or -1 when no process state exists.
func exitCodeOf(hasState bool, code int) int {
	if !hasState {
		return -1
	}
	return code
}
