package evals

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// cursorRunner never runs a subprocess — Cursor's own headless `agent
// -p` prompt/working-directory/model flags are the spec's own open
// "Decides" question. Run always returns ErrHostUnverified;
// RunScenario treats that specific error as Skipped, never as a
// failure. See the plan's design decision 4.
type cursorRunner struct{}

var _ HostRunner = cursorRunner{}

func (cursorRunner) Run(context.Context, exec.UnlistedRunner, *Workspace, string, string) (Transcript, error) {
	return Transcript{}, ErrHostUnverified
}
