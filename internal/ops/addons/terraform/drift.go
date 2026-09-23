package terraform

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// driftPlanFile is the fixed, relative plan-file name Drift asks
// terraform to write, mirroring internal/mutation/gremlins.reportFile's
// own precedent: a deterministic argv for exectest.Scripted fixtures,
// removed via defer on every exit path so it never becomes a stray
// file inside the rendering's own real cache directory.
const driftPlanFile = ".ai-flow-drift.tfplan"

// detailedExitcodeChanges is terraform plan -detailed-exitcode's own
// documented "changes present, no error" exit code.
const detailedExitcodeChanges = 2

// Drift compares baseRender against t's live environment:
// resolveBinding first (§ env.go — env.Name == "" or an environment
// with no terraform binding is ops.ErrLiveCredentialsMissing before
// any subprocess runs; internal/cli/ops_drift.go's own existing code
// degrades that to drift: "offline"), then a real init + workspace
// select, then "plan -detailed-exitcode -out=<tmp>"; exit 0 is
// DriftNone, exit 2 is DriftPresent (every resource_changes[] address
// whose own actions are not exactly ["no-op"], read from
// "show -json <tmp>"), anything else is a real tool_failed.
func (a *Addon) Drift(ctx context.Context, _ ops.Target, env ops.Env, baseRender *ops.Rendered) (*ops.DriftReport, error) {
	b, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}
	if err := a.liveInit(ctx, baseRender.Dir, b); err != nil {
		return nil, err
	}

	planPath := filepath.Join(baseRender.Dir, driftPlanFile)
	defer func() { _ = os.Remove(planPath) }()

	args := append([]string{"plan", "-detailed-exitcode", "-out=" + driftPlanFile, "-input=false", "-no-color"}, planArgs(b)...)
	opts := exec.RunOpts{Dir: baseRender.Dir, Env: a.tfEnv(), Timeout: a.timeout()}
	out, err := a.x.Run(ctx, "terraform", args, opts)
	switch {
	case err == nil:
		return &ops.DriftReport{Drift: ops.DriftNone, Resources: []string{}}, nil
	case exec.IsToolFailed(err) && out.ExitCode == detailedExitcodeChanges:
		resources, showErr := a.driftResources(ctx, baseRender.Dir)
		if showErr != nil {
			return nil, showErr
		}
		return &ops.DriftReport{Drift: ops.DriftPresent, Resources: resources}, nil
	default:
		return nil, err
	}
}

func (a *Addon) driftResources(ctx context.Context, dir string) ([]string, error) {
	plan, err := a.showPlan(ctx, dir, driftPlanFile)
	if err != nil {
		return nil, err
	}

	var resources []string
	for _, rc := range plan.ResourceChanges {
		if rc.Mode == dataMode {
			continue
		}
		kind, kErr := actionKind(rc.Change.Actions)
		if kErr != nil {
			return nil, kErr
		}
		if kind != ops.ActionNoop {
			resources = append(resources, rc.Address)
		}
	}
	if resources == nil {
		resources = []string{}
	}
	return resources, nil
}
