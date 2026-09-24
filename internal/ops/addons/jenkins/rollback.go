package jenkins

import (
	"context"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Rollback derives the revert path as text only, running no
// subprocess (the spec's own words: "produced as text and never
// executed") — identical shape and reasoning to
// githubactions.Rollback. NonRevertible is always empty: every action
// this addon produces carries KindClass "pipeline", never one of the
// four non-revertible classes.
func (a *Addon) Rollback(_ context.Context, t ops.Target, _ ops.Env, _ *ops.Changeset) (*ops.RollbackInfo, error) {
	return &ops.RollbackInfo{
		Steps: []string{
			fmt.Sprintf("git revert the commits touching %s, then confirm the previous job config with `ops drift` before trusting it again.", t.Path),
		},
	}, nil
}
