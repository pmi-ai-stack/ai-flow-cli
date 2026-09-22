// Package exectest provides a scripted exec.Execer test double, shared
// by every package that needs to prove its subprocess calls without
// ever running a real one — found during phase 5's /simplify pass,
// when internal/gitops and internal/cli had each grown their own
// independent copy of the same double. Mirrors
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
	Tool string
	Args []string
	Dir  string
}

// Scripted is a scripted exec.Execer: responses are matched by the
// joined argv (tool + args), first match wins, so a test registers
// exactly the calls it expects and fails loudly (via t.Fatalf) on
// anything unregistered — the assertion mechanism for "no such call
// happened" cases.
type Scripted struct {
	t         *testing.T
	responses map[string]exec.Output
	fail      map[string]error
	Calls     []Call
}

var _ exec.Execer = (*Scripted)(nil)

// New builds an empty Scripted double.
func New(t *testing.T) *Scripted {
	t.Helper()
	return &Scripted{t: t, responses: map[string]exec.Output{}, fail: map[string]error{}}
}

func key(tool string, args []string) string {
	return tool + " " + strings.Join(args, " ")
}

// On registers out as the response for tool+args.
func (s *Scripted) On(tool string, args []string, out exec.Output) *Scripted {
	s.responses[key(tool, args)] = out
	return s
}

// OnFail registers err as the response for tool+args.
func (s *Scripted) OnFail(tool string, args []string, err error) *Scripted {
	s.fail[key(tool, args)] = err
	return s
}

// Run implements exec.Execer against the registered script.
func (s *Scripted) Run(_ context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	s.t.Helper()
	s.Calls = append(s.Calls, Call{Tool: tool, Args: args, Dir: opts.Dir})
	k := key(tool, args)
	if err, ok := s.fail[k]; ok {
		return exec.Output{ExitCode: 1}, err
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
