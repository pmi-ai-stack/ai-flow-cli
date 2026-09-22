package exectest

import (
	"context"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// UnlistedCall is one recorded UnlistedScripted.RunUnlisted invocation.
type UnlistedCall struct {
	Tool string
	Args []string
	Dir  string
	Env  []string
}

// UnlistedScripted is a scripted exec.UnlistedRunner test double —
// Scripted's own role for the "outside the allowlist" exception
// (internal/verify/visual's dev server, internal/skills/evals's
// headless host and grader calls). Responses are consumed in queued
// order via Then/ThenFail, not matched by argv the way Scripted
// matches Execer.Run calls: a caller's own argv can embed large,
// per-call-varying content (a grading prompt's full transcript text,
// for instance) that isn't practical to match verbatim in a test's own
// literal, so each test registers its responses as an ordered queue
// instead. Added during a /simplify pass after internal/skills/evals
// and internal/cli had each grown their own independent copy of this
// same double — the identical history Scripted's own doc comment
// records for the allowlisted path.
type UnlistedScripted struct {
	t       *testing.T
	outputs []exec.Output
	errs    []error
	i       int
	Calls   []UnlistedCall
}

var _ exec.UnlistedRunner = (*UnlistedScripted)(nil)

// NewUnlisted builds an empty UnlistedScripted double.
func NewUnlisted(t *testing.T) *UnlistedScripted {
	t.Helper()
	return &UnlistedScripted{t: t}
}

// Then queues out (with a nil error) as the next RunUnlisted response.
func (s *UnlistedScripted) Then(out exec.Output) *UnlistedScripted {
	return s.ThenFail(out, nil)
}

// ThenFail queues out and err together as the next RunUnlisted
// response — a subprocess that ran, produced captured output, and
// still returned a non-nil error (the allowlisted path's
// OnFailWithOutput has the same shape, for the same reason).
func (s *UnlistedScripted) ThenFail(out exec.Output, err error) *UnlistedScripted {
	s.outputs = append(s.outputs, out)
	s.errs = append(s.errs, err)
	return s
}

// RunUnlisted implements exec.UnlistedRunner against the queued
// script.
func (s *UnlistedScripted) RunUnlisted(_ context.Context, dir, tool string, args, env []string, _ time.Duration) (exec.Output, error) {
	s.t.Helper()
	s.Calls = append(s.Calls, UnlistedCall{Tool: tool, Args: args, Dir: dir, Env: env})
	if s.i >= len(s.outputs) {
		s.t.Fatalf("exectest: unexpected UnlistedRunner call #%d: %s %v", s.i, tool, args)
	}
	out, err := s.outputs[s.i], s.errs[s.i]
	s.i++
	return out, err
}

// StartUnlisted is not scripted — no caller of UnlistedScripted uses
// the start-now-stop-later shape (that is internal/verify/visual's own
// DevServer, tested against its own fakeUnlistedRunner).
func (s *UnlistedScripted) StartUnlisted(context.Context, string, string, []string, []string) (*exec.Process, error) {
	s.t.Fatalf("exectest: StartUnlisted is not scripted by UnlistedScripted")
	return nil, nil
}
