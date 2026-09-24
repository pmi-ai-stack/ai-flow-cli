package kustomize

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
		"staging": {Tier: "nonprod"}, // no Kube binding
	}}}
	if _, err := resolveBinding(cfg, "staging"); !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("resolveBinding(staging) error = %v, want ops.ErrLiveCredentialsMissing (no kube binding configured)", err)
	}
}

func TestResolveBindingReturnsConfiguredContext(t *testing.T) {
	b, err := resolveBinding(boundConfig(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if b.context != "prod-ctx" {
		t.Errorf("binding.context = %q, want prod-ctx", b.context)
	}
}

func TestContextFlagEmptyWhenNoContext(t *testing.T) {
	if got := (binding{}).contextFlag(); got != nil {
		t.Errorf("contextFlag() = %v, want nil for an empty context", got)
	}
}

func TestContextFlagNamesContext(t *testing.T) {
	got := (binding{context: "prod-ctx"}).contextFlag()
	want := []string{"--context", "prod-ctx"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("contextFlag() = %v, want %v", got, want)
	}
}
