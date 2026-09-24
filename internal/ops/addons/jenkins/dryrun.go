package jenkins

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// DryRun mirrors terraform's/kubectl's own live-vs-rendering shape —
// the architecturally consistent reading once it became clear
// Addon.DryRun's own frozen signature never receives a base rendering
// (only Drift does — see drift.go and the phase-27 plan's own design
// decision 5): resolveBinding first, then the identical live
// "jenkins-cli get-job" call Drift makes, but compared against r (the
// **branch** rendering) instead of baseRender. Drift looks backward
// (live vs base, "has this drifted from what's checked in"), DryRun
// looks forward (live vs branch, "what would this change do") — the
// same split every other real addon's Drift/DryRun pair already has.
// A "no such job" failure here is read as ActionCreate — a brand-new
// target with nothing live yet is the normal, expected case DryRun
// must not treat as a hard failure the way Drift's own equivalent case
// is left unresolved (§1.2.2 of the phase-27 plan) — everything else
// jenkins-cli reports is a genuine tool_failed, propagated unchanged.
func (a *Addon) DryRun(ctx context.Context, t ops.Target, env ops.Env, r *ops.Rendered) (*ops.Changeset, error) {
	url, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}

	live, jobExists, err := a.getJobTolerant(ctx, url, t)
	if err != nil {
		return nil, err
	}

	branch, err := readRendering(r.Dir)
	if err != nil {
		return nil, err
	}

	var action ops.Action
	switch {
	case !jobExists:
		action = ops.Action{Action: ops.ActionCreate}
	case normalizeText(live) == normalizeText(branch):
		return &ops.Changeset{}, nil
	default:
		action = ops.Action{Action: ops.ActionUpdate, FieldsChanged: []string{"content"}}
	}

	action.Address = t.Path
	action.Addon = "jenkins"
	action.Env = env.Name
	action.Kind = targetKind(t.Path)
	action.KindClass = ops.KindPipeline
	action.Name = jobName(t.Repo, t.Path)
	action.Target = ops.TargetKey(t)
	action.Tier = env.Tier
	return &ops.Changeset{Actions: []ops.Action{action}}, nil
}

// getJobTolerant runs jenkins-cli get-job, reading any tool_failed
// exit (the shape a "no such job" failure takes — jenkins-cli's own
// exact contract is unverified against a real server, doc.go) as
// "this job does not exist live yet," never a hard error: DryRun must
// not fail on the ordinary, expected case of a brand-new target with
// nothing deployed yet. A non-tool_failed error (exec_not_allowed, a
// real internal error) still propagates.
func (a *Addon) getJobTolerant(ctx context.Context, url string, t ops.Target) (content string, exists bool, err error) {
	live, err := a.getJob(ctx, url, t)
	switch {
	case err == nil:
		return live, true, nil
	case exec.IsToolFailed(err):
		return "", false, nil
	default:
		return "", false, err
	}
}
