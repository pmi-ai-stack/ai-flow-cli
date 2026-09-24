package githubactions

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Drift satisfies ops.Addon but is never called in production:
// Capabilities().Drift is false (addon.go's own doc comment explains
// why — internal/cli/ops_drift.go's driftOneTarget already defaults
// every target to DriftNotApplicable and only calls this method when
// the capability is true). A workflow file has no live counterpart at
// all, so there is nothing this method could honestly compare
// baseRender against even if it were called.
func (a *Addon) Drift(_ context.Context, _ ops.Target, _ ops.Env, _ *ops.Rendered) (*ops.DriftReport, error) {
	return &ops.DriftReport{Drift: ops.DriftNotApplicable, Resources: []string{}}, nil
}
