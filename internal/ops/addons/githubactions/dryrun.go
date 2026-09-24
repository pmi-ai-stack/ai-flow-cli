package githubactions

import (
	"context"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// DryRun: "act -n -W <file>" against r (the branch rendering) as a
// real job-graph confirmation (verified for real against a clean and
// a deliberately broken workflow, the phase-27 plan's own §0.1) — a
// failure is a genuine tool_failed, propagated unchanged. The
// "structural diff" half of the matrix's own DryRun cell is, for this
// addon specifically, a single per-target Action naming the job ids
// the branch rendering currently defines, never a base-vs-branch
// delta: unlike every other addon, Addon.DryRun's own frozen signature
// receives no base rendering (only Drift does, via its own baseRender
// parameter — and this addon's own Drift is not_applicable, since a
// workflow file has no live counterpart to compare base against
// either). DryRun is only ever called for a target internal/cli
// already knows changed (the should-run/target-scoping rule every
// stage shares), so a single ActionUpdate per target reports the fact
// honestly without inventing a delta this addon's own interface
// surface cannot compute — see .ai/phase-27/plan.md's own corrected
// design decision.
func (a *Addon) DryRun(ctx context.Context, t ops.Target, env ops.Env, r *ops.Rendered) (*ops.Changeset, error) {
	if _, err := a.x.Run(ctx, "act", []string{"-n", "-W", RenderedFile}, exec.RunOpts{Dir: r.Dir, Timeout: a.timeout()}); err != nil {
		return nil, err
	}

	jobs, err := workflowJobs(filepath.Join(r.Dir, RenderedFile))
	if err != nil {
		return nil, err
	}
	ids := sortedJobIDs(jobs)

	return &ops.Changeset{Actions: []ops.Action{{
		Action: ops.ActionUpdate, Address: t.Path, Addon: "github-actions",
		Env: env.Name, Kind: "workflow", KindClass: ops.KindPipeline,
		Name: t.Path, FieldsChanged: ids, Target: ops.TargetKey(t), Tier: env.Tier,
	}}}, nil
}
