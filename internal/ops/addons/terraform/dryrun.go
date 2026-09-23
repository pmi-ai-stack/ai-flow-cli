package terraform

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// dryRunPlanFile is DryRun's own fixed plan-file name — driftPlanFile's
// sibling, kept distinct so a Drift and a DryRun call against the same
// rendering directory (which does happen: Drift plans the base
// rendering, DryRun the branch rendering, but a caller could point
// both at the same Target/ref in a test) never race on one file name.
const dryRunPlanFile = ".ai-flow-dryrun.tfplan"

// DryRun resolves t's binding (env.Name == "" or unbound is
// ops.ErrLiveCredentialsMissing, § env.go — internal/cli/ops_dryrun.go's
// own existing code turns that into the documented
// live_credentials_missing envelope error), runs the same real
// init/workspace-select sequence Drift uses against r (the branch
// rendering), "plan -out=<tmp>" (no -detailed-exitcode — this call
// only wants the plan file, not its own exit-code semantics), then
// "show -json <tmp>" normalized via buildChangeset.
func (a *Addon) DryRun(ctx context.Context, t ops.Target, env ops.Env, r *ops.Rendered) (*ops.Changeset, error) {
	b, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}
	if err := a.liveInit(ctx, r.Dir, b); err != nil {
		return nil, err
	}

	planPath := filepath.Join(r.Dir, dryRunPlanFile)
	defer func() { _ = os.Remove(planPath) }()

	args := append([]string{"plan", "-out=" + dryRunPlanFile, "-input=false", "-no-color"}, planArgs(b)...)
	opts := exec.RunOpts{Dir: r.Dir, Env: a.tfEnv(), Timeout: a.timeout()}
	if _, err := a.x.Run(ctx, "terraform", args, opts); err != nil {
		return nil, err
	}

	plan, err := a.showPlan(ctx, r.Dir, dryRunPlanFile)
	if err != nil {
		return nil, err
	}

	cs, err := buildChangeset(plan, t, env)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}
