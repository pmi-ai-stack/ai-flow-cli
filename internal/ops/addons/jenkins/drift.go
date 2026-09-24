package jenkins

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Drift: resolveBinding first (env.Name == "" or an environment with
// no jenkins binding is ops.ErrLiveCredentialsMissing, exactly like
// terraform's/kubectl's own Drift); then the real, read-only
// "jenkins-cli get-job <name> -s <url>" call (the matrix's own literal
// cell — provisional, unverified against a real server, doc.go), its
// output compared against baseRender's own file content
// (whitespace-normalized — normalize.go). A "no such job" failure is
// a genuine tool_failed here, propagated unchanged: unlike DryRun
// (dryrun.go), a target with nothing live yet is a real, disclosed
// limitation of this addon's Drift, not a case this phase has a
// verified way to distinguish from any other jenkins-cli failure
// (§1.2.2 of the phase-27 plan).
func (a *Addon) Drift(ctx context.Context, t ops.Target, env ops.Env, baseRender *ops.Rendered) (*ops.DriftReport, error) {
	url, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}

	live, err := a.getJob(ctx, url, t)
	if err != nil {
		return nil, err
	}

	base, err := readRendering(baseRender.Dir)
	if err != nil {
		return nil, err
	}

	if normalizeText(live) == normalizeText(base) {
		return &ops.DriftReport{Drift: ops.DriftNone, Resources: []string{}}, nil
	}
	return &ops.DriftReport{Drift: ops.DriftPresent, Resources: []string{jobName(t.Repo, t.Path)}}, nil
}

// getJob runs jenkins-cli get-job for real, returning its own stdout
// (the job's live config content).
func (a *Addon) getJob(ctx context.Context, url string, t ops.Target) (string, error) {
	args := append([]string{"get-job", jobName(t.Repo, t.Path), "-s", url}, jenkinsAuthArgs()...)
	out, err := a.x.Run(ctx, "jenkins-cli", args, exec.RunOpts{Timeout: a.timeout()})
	if err != nil {
		return "", err
	}
	return out.Stdout, nil
}
