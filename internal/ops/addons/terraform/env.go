package terraform

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// binding is one environment's resolved terraform workspace/var-file
// pair — config.ops.environments[name].terraform, looked up by name
// since ops.Env itself carries only Name and Tier (the frozen Addon
// interface, phase 18 on).
type binding struct {
	workspace string
	varFile   string
}

// resolveBinding returns env's own terraform binding, or
// ops.ErrLiveCredentialsMissing when name is unbound ("") or bound to
// an environment with no terraform binding configured — the phase-21
// plan's own design decision 2: terraform's plan computation *is* its
// live comparison, with no meaningful offline degradation, so "no
// binding to plan against" is always treated as "live credentials
// missing," unconditionally, before any subprocess ever runs. A real
// subprocess failure against a properly bound environment is always
// tool_failed instead — this function never tries to reclassify one
// after the fact.
func resolveBinding(cfg *config.Config, name string) (binding, error) {
	if name == "" {
		return binding{}, ops.ErrLiveCredentialsMissing
	}
	env, ok := cfg.Ops.Environments[name]
	if !ok || env.Terraform == nil {
		return binding{}, ops.ErrLiveCredentialsMissing
	}
	return binding{workspace: env.Terraform.Workspace, varFile: env.Terraform.VarFile}, nil
}

// liveInit runs the sequence Drift and DryRun both need before
// planning: a real (backend-reading) init, then a workspace switch
// when b names one. dir is the rendering's own real cache directory
// (already initialized with -backend=false by Render — this only adds
// backend/state wiring on top of an already-valid module, the plan's
// own design decision 5).
func (a *Addon) liveInit(ctx context.Context, dir string, b binding) error {
	opts := exec.RunOpts{Dir: dir, Env: a.tfEnv(), Timeout: a.timeout()}
	if _, err := a.x.Run(ctx, "terraform", []string{"init", "-input=false", "-no-color"}, opts); err != nil {
		return err
	}
	if b.workspace == "" {
		return nil
	}
	_, err := a.x.Run(ctx, "terraform", []string{"workspace", "select", b.workspace}, opts)
	return err
}

// planArgs builds a "terraform plan" invocation's own trailing flags
// for b — var-file only when configured, so a target with no var file
// still plans against its own defaults.
func planArgs(b binding) []string {
	if b.varFile == "" {
		return nil
	}
	return []string{"-var-file=" + b.varFile}
}

// showPlan runs "terraform show -json <planFile>" in dir and parses
// its output — the one step Drift (reading changed addresses off it)
// and DryRun (building a full Changeset off it) both need after their
// own "plan -out=<planFile>" call.
func (a *Addon) showPlan(ctx context.Context, dir, planFile string) (*planJSON, error) {
	out, err := a.x.Run(ctx, "terraform", []string{"show", "-json", planFile}, exec.RunOpts{Dir: dir, Timeout: a.timeout()})
	if err != nil {
		return nil, err
	}
	return parsePlanJSON([]byte(out.Stdout))
}
