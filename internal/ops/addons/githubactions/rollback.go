package githubactions

import (
	"context"
	"fmt"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Rollback derives the revert path as text only, running no subprocess
// at all (the spec's own words: "produced as text and never
// executed" — mirroring terraform.Rollback's own zero-subprocess
// shape). NonRevertible is always empty: every action this addon ever
// produces carries KindClass "pipeline", never one of the four
// non-revertible classes ("data", "iam", "secret", "crd_or_namespace")
// the spec's own protected/non-revertible formulas name.
func (a *Addon) Rollback(_ context.Context, t ops.Target, _ ops.Env, _ *ops.Changeset) (*ops.RollbackInfo, error) {
	return &ops.RollbackInfo{
		Steps: []string{
			fmt.Sprintf("git revert the commits touching %s, then confirm the workflow's own job graph with `ops dry-run` before trusting it again.", t.Path),
		},
	}, nil
}
