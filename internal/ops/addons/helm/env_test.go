package helm

import (
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func boundConfig() *config.Config {
	return &config.Config{Ops: config.Ops{Environments: map[string]config.Environment{
		"prod": {Tier: "prod", Kube: &config.KubeBinding{Context: "prod-ctx"}},
	}}}
}

func TestResolveBindingUnboundName(t *testing.T) {
	if _, err := resolveBinding(&config.Config{}, ""); !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("resolveBinding(\"\") error = %v, want ops.ErrLiveCredentialsMissing", err)
	}
}

func TestResolveBindingUnconfiguredEnvironment(t *testing.T) {
	cfg := &config.Config{Ops: config.Ops{Environments: map[string]config.Environment{
		"staging": {Tier: "nonprod"},
	}}}
	if _, err := resolveBinding(cfg, "staging"); !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("resolveBinding(staging) error = %v, want ops.ErrLiveCredentialsMissing", err)
	}
}

func TestKubeContextFlagAndContextFlag(t *testing.T) {
	b := binding{context: "prod-ctx"}
	if got := b.kubeContextFlag(); len(got) != 2 || got[0] != "--kube-context" || got[1] != "prod-ctx" {
		t.Errorf("kubeContextFlag() = %v, want [--kube-context prod-ctx]", got)
	}
	if got := b.contextFlag(); len(got) != 2 || got[0] != "--context" || got[1] != "prod-ctx" {
		t.Errorf("contextFlag() = %v, want [--context prod-ctx]", got)
	}
	if got := (binding{}).kubeContextFlag(); got != nil {
		t.Errorf("kubeContextFlag() = %v, want nil for an empty context", got)
	}
}
