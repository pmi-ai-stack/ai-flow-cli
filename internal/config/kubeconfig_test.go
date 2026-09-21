package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestKubeconfigNoWarningWhenNoContextNamed(t *testing.T) {
	cfg := validConfig() // no environment names a kube.context
	warnings, err := Validate(cfg, filepath.Join("testdata", "kubeconfig-sample.yaml"))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none when no environment names a kube context", warnings)
	}
}

func TestKubeconfigNoWarningWhenContextPresent(t *testing.T) {
	cfg := validConfig()
	cfg.Ops.Environments["dev"] = Environment{Tier: "nonprod", Kube: &KubeBinding{Context: "dev-ro"}}

	warnings, err := Validate(cfg, filepath.Join("testdata", "kubeconfig-sample.yaml"))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none: dev-ro is present in the fixture kubeconfig", warnings)
	}
}

func TestKubeconfigWarningOnUnknownContext(t *testing.T) {
	cfg := validConfig()
	cfg.Ops.Environments["dev"] = Environment{Tier: "nonprod", Kube: &KubeBinding{Context: "no-such-context"}}

	warnings, err := Validate(cfg, filepath.Join("testdata", "kubeconfig-sample.yaml"))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no-such-context") {
		t.Errorf("warnings = %v, want one warning naming no-such-context", warnings)
	}
}

func TestKubeconfigWarningOnMissingFile(t *testing.T) {
	cfg := validConfig()
	cfg.Ops.Environments["dev"] = Environment{Tier: "nonprod", Kube: &KubeBinding{Context: "dev-ro"}}

	warnings, err := Validate(cfg, filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Validate: %v (a missing kubeconfig must be a warning, never a validation failure)", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unreadable") {
		t.Errorf("warnings = %v, want one \"unreadable\" warning", warnings)
	}
}

func TestKubeconfigCheckSkippedWhenPathEmpty(t *testing.T) {
	cfg := validConfig()
	cfg.Ops.Environments["dev"] = Environment{Tier: "nonprod", Kube: &KubeBinding{Context: "no-such-context"}}

	warnings, err := Validate(cfg, "")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none when kubeconfigPath is empty", warnings)
	}
}
