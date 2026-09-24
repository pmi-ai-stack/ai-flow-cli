package helm

import (
	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// binding is one environment's resolved kubectl context —
// config.ops.environments[name].kube, the same binding type
// kubectl/kustomize use.
type binding struct {
	context string
}

// resolveBinding returns env's own kube binding, or
// ops.ErrLiveCredentialsMissing when name is unbound ("") or bound to
// an environment with no kube binding configured — a helm target's
// live comparison (helm get manifest/diff upgrade/history) is itself
// the credential check, so there is no meaningful offline degradation,
// the same reasoning kubectl's/terraform's own resolveBinding already
// established.
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

// kubeContextFlag returns b's own "--kube-context <name>" argument
// pair, or nil when no context is configured — helm's own flag name
// for what kubectl calls "--context".
func (b binding) kubeContextFlag() []string {
	if b.context == "" {
		return nil
	}
	return []string{"--kube-context", b.context}
}

// contextFlag returns b's own "--context <name>" argument pair for the
// kubectl-flavoured fallback calls (DryRun's "template + kubectl diff"
// path) — kubectl's own flag name, distinct from helm's.
func (b binding) contextFlag() []string {
	if b.context == "" {
		return nil
	}
	return []string{"--context", b.context}
}
