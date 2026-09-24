package kustomize

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// binding is one environment's resolved kubectl context —
// config.ops.environments[name].kube, looked up by name since ops.Env
// itself carries only Name and Tier (the frozen Addon interface).
// Identical shape to kubectl's own — both addons read the same
// KubeBinding config type.
type binding struct {
	context string
}

// resolveBinding returns env's own kube binding, or
// ops.ErrLiveCredentialsMissing when name is unbound ("") or bound to
// an environment with no kube binding configured — kubectl's own
// phase-22 reasoning applied unchanged: a kustomize target's live
// comparison (kubectl diff/get/apply) is itself the credential check,
// so there is no meaningful offline degradation.
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
// when no context is configured.
func (b binding) contextFlag() []string {
	if b.context == "" {
		return nil
	}
	return []string{"--context", b.context}
}

// getLiveByAddress runs "kubectl get -f . -o json --ignore-not-found"
// in dir and returns the result indexed by address — shared by Drift's
// driftResources and DryRun's buildChangedChangeset, identical to
// kubectl's own helper.
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
