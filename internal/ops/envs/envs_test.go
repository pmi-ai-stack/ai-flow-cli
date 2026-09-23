package envs_test

import (
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/envs"
)

func testConfig() *config.Config {
	return &config.Config{
		Ops: config.Ops{
			Environments: map[string]config.Environment{
				"dev":  {Tier: "nonprod"},
				"prod": {Tier: "prod"},
			},
			TargetMap:          map[string]string{"acme.k8s-config:overlays/eu-prod": "prod"},
			DefaultEnvironment: "dev",
		},
	}
}

func TestBindTargetMapWins(t *testing.T) {
	cfg := testConfig()
	got, err := envs.Bind(cfg, "acme.k8s-config", "overlays/eu-prod", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != (envs.Result{Name: "prod", Tier: "prod"}) {
		t.Errorf("Bind = %+v, want {prod prod}", got)
	}
}

func TestBindEnvsConvention(t *testing.T) {
	cfg := testConfig()
	got, err := envs.Bind(cfg, "acme.infra", "envs/prod/main.tf", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != (envs.Result{Name: "prod", Tier: "prod"}) {
		t.Errorf("Bind = %+v, want {prod prod}", got)
	}
}

func TestBindOverlaysConvention(t *testing.T) {
	cfg := testConfig()
	got, err := envs.Bind(cfg, "acme.infra", "overlays/dev/kustomization.yaml", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != (envs.Result{Name: "dev", Tier: "nonprod"}) {
		t.Errorf("Bind = %+v, want {dev nonprod}", got)
	}
}

func TestBindValuesFileConventionMatchesAgainstFiles(t *testing.T) {
	cfg := testConfig()
	got, err := envs.Bind(cfg, "acme.charts", "charts/app", []string{"charts/app/values-prod.yaml"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != (envs.Result{Name: "prod", Tier: "prod"}) {
		t.Errorf("Bind = %+v, want {prod prod}", got)
	}
}

func TestBindDefaultEnvironmentFallback(t *testing.T) {
	cfg := testConfig()
	got, err := envs.Bind(cfg, "acme.infra", "infra/shared", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != (envs.Result{Name: "dev", Tier: "nonprod"}) {
		t.Errorf("Bind = %+v, want the default environment {dev nonprod}", got)
	}
}

func TestBindUnbound(t *testing.T) {
	cfg := testConfig()
	got, err := envs.Bind(cfg, "acme.actions", ".github/workflows/ci.yml", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != (envs.Result{Name: "", Tier: "nonprod"}) {
		t.Errorf("Bind = %+v, want the unbound environment {\"\" nonprod}", got)
	}
	if got.Name != "" {
		t.Error("unbound environment must report an empty Name")
	}
}

func TestBindAmbiguousWhenTwoDifferentConventionsMatchTwoDifferentEnvironments(t *testing.T) {
	cfg := testConfig()
	// The path matches "prod" via envs/<name>/; one of the target's own
	// changed files matches "dev" via values-<name>.yaml. Two different
	// rules, two different environments -> ambiguous.
	_, err := envs.Bind(cfg, "acme.infra", "envs/prod/main.tf", []string{"envs/prod/values-dev.yaml"}, true)
	if err == nil {
		t.Fatal("Bind = nil error, want env_ambiguous")
	}
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("error is not *envelope.Error: %v", err)
	}
	if envErr.Code != "env_ambiguous" {
		t.Errorf("Code = %q, want env_ambiguous", envErr.Code)
	}
	want := []string{"dev", "prod"}
	if len(envErr.Details) != 2 || envErr.Details[0] != want[0] || envErr.Details[1] != want[1] {
		t.Errorf("Details = %v, want %v (sorted)", envErr.Details, want)
	}
}

func TestBindTargetMapNeverAmbiguous(t *testing.T) {
	// Even though the path would also match a convention, an explicit
	// target_map entry short-circuits before convention matching runs.
	cfg := testConfig()
	cfg.Ops.TargetMap["acme.infra:envs/prod/main.tf"] = "dev"
	got, err := envs.Bind(cfg, "acme.infra", "envs/prod/main.tf", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "dev" {
		t.Errorf("Bind = %+v, want target_map's own answer (dev), not the convention match (prod)", got)
	}
}
