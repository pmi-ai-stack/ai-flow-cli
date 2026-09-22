package evals

import (
	"context"
	"errors"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// hostTimeout bounds one headless host session — generous enough for a
// real agentic run, short enough that a hung subprocess does not stall
// a CI job indefinitely. No config surface names a value for this
// (skills evals is a CI-time tool, not a pipeline stage), matching
// internal/cli/visual.go's own captureTimeout precedent for the same
// kind of fixed, undocumented-in-config default.
const hostTimeout = 10 * time.Minute

// Transcript is one headless session's captured output — raw text,
// since neither host's real `-p` output shape is contract this
// codebase controls (Grade extracts what it needs from it directly,
// rather than this package committing to a parsed structure it cannot
// verify against a real product from this environment — see the
// plan's §1.3).
type Transcript struct {
	Text     string
	ExitCode int
}

// HostRunner is EvalRunner's own per-host half (the other half is
// Grade, which every host shares) — internal/skills/evals/<host>.go
// per the spec's own Interfaces (Go) comment.
type HostRunner interface {
	Run(ctx context.Context, x exec.UnlistedRunner, w *Workspace, query, model string) (Transcript, error)
}

// Runners is every registered host, by the same name --host accepts.
var Runners = map[string]HostRunner{
	"claude-code": claudeCodeRunner{},
	"cursor":      cursorRunner{},
}

// ErrHostUnverified is cursorRunner's own sentinel — RunScenario
// treats it as Skipped, never as a failure, per the plan's design
// decision 4.
var ErrHostUnverified = errors.New("evals: agent -p headless flags are not verified (spec's own open Decides question) — the cursor eval column is off")
