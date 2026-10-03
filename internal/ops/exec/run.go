package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// RunOpts carries a subprocess's working directory, extra environment
// variables (appended to the inherited environment, never replacing
// it), its timeout and (phase 27) its stdin content. A zero Timeout
// means no timeout is applied — every production call site sets one
// from config.ops.tool_timeout_minutes. Stdin is empty for every call
// site before phase 27; jenkins-cli's own "declarative-linter"
// subcommand is the first tool in this codebase's history that reads
// its input from stdin rather than a file path or a flag (Jenkins's
// own documented CLI contract — see internal/ops/addons/jenkins's own
// Validate) — a small, additive field, not a redesign of the
// execution path (the phase-27 plan's own §0.6).
type RunOpts struct {
	Dir     string
	Env     []string
	Stdin   string
	Timeout time.Duration
}

// Output is a subprocess's captured, redacted result. It is returned
// alongside a non-nil error on a non-zero exit (not just on success),
// so a caller that needs to tell "the tool reported an expected
// negative result" (e.g. a ref that does not exist yet) from a real
// failure can inspect ExitCode without re-parsing Stderr.
type Output struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// IsToolFailed reports whether err is the tool_failed error Run (or
// RunUnlisted) returns for a non-zero exit — the signal a caller uses
// to tell "the tool ran and reported a real result" (Output is still
// populated, see Output's own doc comment) from "the tool never ran at
// all" (e.g. exec_not_allowed), which needs different handling: a
// runner that exits non-zero because its own suite failed should still
// have its Output parsed, not be treated as an internal CLI error.
func IsToolFailed(err error) bool {
	var envErr *envelope.Error
	return errors.As(err, &envErr) && envErr.Code == "tool_failed"
}

// NotFoundError is the tool_failed error Run returns when tool is not
// on PATH at all. It is still tool_failed to every existing caller —
// errors.As to *envelope.Error yields Err, so IsToolFailed, the
// envelope and the exit code are exactly what they were — and
// IsNotFound lets the one caller that must tell "the tool ran and found
// nothing" from "the tool never ran" (standards check, where the two
// otherwise both look like an empty report) do so.
type NotFoundError struct {
	Tool string
	Err  *envelope.Error
}

func (e *NotFoundError) Error() string { return e.Err.Error() }

// As lets errors.As(err, **envelope.Error) see through to Err.
func (e *NotFoundError) As(target any) bool {
	if p, ok := target.(**envelope.Error); ok {
		*p = e.Err
		return true
	}
	return false
}

// NotFound builds the error Run returns for a tool missing from PATH —
// exported so a fake Execer can report the same condition.
func NotFound(tool string) error {
	return &NotFoundError{Tool: tool, Err: &envelope.Error{
		Code:    "tool_failed",
		Message: fmt.Sprintf("%s: executable file not found in $PATH", tool),
	}}
}

// IsNotFound reports whether err says the tool was not on PATH, and
// which tool.
func IsNotFound(err error) (string, bool) {
	var nf *NotFoundError
	if errors.As(err, &nf) {
		return nf.Tool, true
	}
	return "", false
}

// Execer is how every package outside this one runs a subprocess.
// internal/ops/exec is the only package allowed to import os/exec
// (TestNoOsExecOutsideExecPackage) — Execer is the seam that lets a
// caller's own tests inject a fake instead of running a real tool.
type Execer interface {
	Run(ctx context.Context, tool string, args []string, opts RunOpts) (Output, error)
}

// Runner is the production Execer: every call is matched against an
// allowlist first — a miss is exec_not_allowed and no subprocess ever
// starts — then run with output captured and redacted before it ever
// leaves this package.
type Runner struct {
	Allowlist *Allowlist
	Redact    *redact.Redactor
}

var _ Execer = (*Runner)(nil)

// NewRunner builds the production Runner: Table as the allowlist,
// secrets discovered from the real process environment.
func NewRunner() *Runner {
	return &Runner{Allowlist: NewAllowlist(Table...), Redact: redact.New(os.Environ())}
}

// Run matches tool+args against r.Allowlist, denies with
// exec_not_allowed on a miss, else runs it with opts.Dir and
// opts.Timeout, capturing and redacting stdout/stderr. A non-zero exit
// is reported as a tool_failed *envelope.Error carrying the redacted
// stderr, with Output still populated (see Output's own doc comment).
func (r *Runner) Run(ctx context.Context, tool string, args []string, opts RunOpts) (Output, error) {
	argv := append([]string{tool}, args...)
	if _, ok := r.Allowlist.Match(argv); !ok {
		return Output{}, NotAllowed(argv)
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := osexec.CommandContext(ctx, tool, args...) // tool+args are matched against the allowlist above first (G204 excluded module-wide)
	cmd.Dir = opts.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	out := Output{
		Stdout: r.Redact.String(stdout.String()),
		Stderr: r.Redact.String(stderr.String()),
	}
	if cmd.ProcessState != nil {
		out.ExitCode = cmd.ProcessState.ExitCode()
	}
	if runErr != nil {
		failed := &envelope.Error{
			Code:    "tool_failed",
			Message: fmt.Sprintf("%s: %v: %s", strings.Join(argv, " "), runErr, strings.TrimSpace(out.Stderr)),
		}
		if errors.Is(runErr, osexec.ErrNotFound) {
			return out, &NotFoundError{Tool: tool, Err: failed}
		}
		return out, failed
	}
	return out, nil
}
