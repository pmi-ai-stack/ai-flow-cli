package kubectl

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// binding is one environment's resolved kubectl context —
// config.ops.environments[name].kube, looked up by name since ops.Env
// itself carries only Name and Tier (the frozen Addon interface).
type binding struct {
	context string
}

// resolveBinding returns env's own kube binding, or
// ops.ErrLiveCredentialsMissing when name is unbound ("") or bound to
// an environment with no kube binding configured. Unlike an addon
// whose targets have a meaningful offline mode, kubectl's diff/get/
// apply --dry-run=server calls are themselves the live comparison —
// server-side apply needs the live API server to compute defaulting
// and admission — so "no binding to compare against" is always
// treated as "live credentials missing," unconditionally, before any
// subprocess ever runs (the same rule terraform's own resolveBinding
// already established, phase 21's env.go, design decision 2). A real
// subprocess failure against a properly bound environment is always
// tool_failed instead.
func resolveBinding(cfg *config.Config, name string) (binding, error) {
	if name == "" {
		return binding{}, ops.ErrLiveCredentialsMissing
	}
	env, ok := cfg.Ops.Environments[name]
	if !ok || env.Kube == nil {
		return binding{}, ops.ErrLiveCredentialsMissing
	}
	return binding{context: env.Kube.Context}, nil
}

// contextFlag returns b's own "--context <name>" argument pair, or nil
// when no context is configured (an environment may bind kubectl with
// no explicit context, deferring to whatever kubectl's own current
// context is).
func (b binding) contextFlag() []string {
	if b.context == "" {
		return nil
	}
	return []string{"--context", b.context}
}

// getLiveByAddress runs "kubectl get -f . -o json --ignore-not-found"
// in dir and returns the result indexed by address — the "read live
// state" step both Drift's driftResources and DryRun's
// buildChangedChangeset need for their own before/live comparison,
// shared here rather than each independently building the same argv
// and repeating the same decode-then-index steps.
func (a *Addon) getLiveByAddress(ctx context.Context, dir string, b binding) (map[string]map[string]any, error) {
	getArgs := append([]string{"get", "-f", ".", "-o", "json", "--ignore-not-found"}, b.contextFlag()...)
	out, err := a.x.Run(ctx, "kubectl", getArgs, exec.RunOpts{Dir: dir, Timeout: a.timeout()})
	if err != nil {
		return nil, err
	}
	live, err := decodeJSONStream([]byte(out.Stdout))
	if err != nil {
		return nil, err
	}
	return indexByAddress(live), nil
}
