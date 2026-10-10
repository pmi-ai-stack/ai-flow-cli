// Package exectest provides scripted test doubles — Scripted for
// exec.Execer, UnlistedScripted for exec.UnlistedRunner — shared by
// every package that needs to prove its subprocess calls without ever
// running a real one. Scripted was found during phase 5's /simplify
// pass, when internal/gitops and internal/cli had each grown their own
// independent copy of the same double; UnlistedScripted was added the
// same way during phase 11's, when internal/skills/evals and
// internal/cli had each grown their own copy of *that* double. Mirrors
// internal/handoff/handofftest's own pattern: a plain (non-_test.go)
// package so it can be imported from another package's tests.
package exectest

import (
	"context"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Call is one recorded Scripted.Run invocation.
type Call struct {
	Tool  string
	Args  []string
	Dir   string
	Stdin string
	Env   []string
}

// Scripted is a scripted exec.Execer: responses are matched by the
// joined argv (tool + args), first match wins, so a test registers
// exactly the calls it expects and fails loudly (via t.Fatalf) on
// anything unregistered — the assertion mechanism for "no such call
// happened" cases.
// failure pairs an OnFail-registered error with the Output to return
// alongside it — a tool that exited non-zero still produced captured
// output (exec.Runner.Run's own documented behaviour, see its doc
// comment), which internal/verify/unit's Run relies on to parse a
// failing suite's report rather than only ever seeing a clean exit.
type failure struct {
	out exec.Output
	err error
}

type Scripted struct {
	t         *testing.T
	responses map[string]exec.Output
	fail      map[string]failure
	Calls     []Call
}

var _ exec.Execer = (*Scripted)(nil)

// New builds an empty Scripted double.
func New(t *testing.T) *Scripted {
	t.Helper()
	return &Scripted{t: t, responses: map[string]exec.Output{}, fail: map[string]failure{}}
}

func key(tool string, args []string) string {
	return tool + " " + strings.Join(args, " ")
}

// On registers out as the response for tool+args.
func (s *Scripted) On(tool string, args []string, out exec.Output) *Scripted {
	s.responses[key(tool, args)] = out
	return s
}

// OnFail registers err as the response for tool+args, with an empty
// Output — the shape a call that never even started (e.g.
// exec_not_allowed) returns.
func (s *Scripted) OnFail(tool string, args []string, err error) *Scripted {
	s.fail[key(tool, args)] = failure{out: exec.Output{ExitCode: 1}, err: err}
	return s
}

// OnFailWithOutput registers both out and err as tool+args's response —
// a tool that ran, produced real captured output, and still exited
// non-zero (a test runner whose suite failed is the paradigm case).
func (s *Scripted) OnFailWithOutput(tool string, args []string, out exec.Output, err error) *Scripted {
	s.fail[key(tool, args)] = failure{out: out, err: err}
	return s
}

// Run implements exec.Execer against the registered script. Every call
// must also pass the production allowlist (RequireAllowlisted): a
// double that answers what the real Runner would refuse hides a broken
// call site — phase 54 found terraform's dry-run plan refused in
// production behind exactly such a double.
func (s *Scripted) Run(_ context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	s.t.Helper()
	RequireAllowlisted(s.t, tool, args)
	s.Calls = append(s.Calls, Call{Tool: tool, Args: args, Dir: opts.Dir, Stdin: opts.Stdin, Env: opts.Env})
	k := key(tool, args)
	if f, ok := s.fail[k]; ok {
		return f.out, f.err
	}
	if out, ok := s.responses[k]; ok {
		return out, nil
	}
	s.t.Fatalf("exectest: unexpected call %q (dir=%s)", k, opts.Dir)
	return exec.Output{}, nil
}

// CalledWithPrefix reports whether any recorded call's argv starts
// with tool followed by prefix, in order.
func (s *Scripted) CalledWithPrefix(tool string, prefix ...string) bool {
	for _, c := range s.Calls {
		if c.Tool != tool || len(c.Args) < len(prefix) {
			continue
		}
		match := true
		for i, p := range prefix {
			if c.Args[i] != p {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// RequireAllowlisted fails tb when tool+args is not on the production
// allowlist (exec.Table, declared flags included). Every Execer test
// double calls it, so a call site whose argv the real Runner would
// refuse fails its own unit test instead of passing behind the double.
func RequireAllowlisted(tb testing.TB, tool string, args []string) {
	tb.Helper()
	argv := append([]string{tool}, args...)
	if _, ok := exec.NewAllowlist(exec.Table...).Match(argv); !ok {
		tb.Errorf("exectest: %q is not on the production allowlist; the real Runner would refuse it with exec_not_allowed", strings.Join(argv, " "))
	}
}

// Refused is what the real Runner answers before running tool+args:
// nil when the production allowlist matches it, else the same
// exec_not_allowed error. A package-local Execer double returns it
// first, so it refuses exactly what production refuses.
func Refused(tool string, args []string) error {
	argv := append([]string{tool}, args...)
	if _, ok := exec.NewAllowlist(exec.Table...).Match(argv); !ok {
		return exec.NotAllowed(argv)
	}
	return nil
}
