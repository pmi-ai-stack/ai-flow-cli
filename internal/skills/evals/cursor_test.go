package evals

import (
	"context"
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestCursorRunner_AlwaysUnverified(t *testing.T) {
	x := exectest.NewUnlisted(t) // no responses queued — a real call would fail the test

	_, err := cursorRunner{}.Run(context.Background(), x, &Workspace{}, "develop PROJ-1", "claude-sonnet-5")
	if !errors.Is(err, ErrHostUnverified) {
		t.Fatalf("Run() error = %v, want ErrHostUnverified", err)
	}
	if len(x.Calls) != 0 {
		t.Errorf("cursorRunner must never call RunUnlisted, got %d calls", len(x.Calls))
	}
}
