package config

import (
	"strings"
	"testing"
)

func TestValidateAcceptsValidConfig(t *testing.T) {
	warnings, err := Validate(validConfig(), "")
	if err != nil {
		t.Fatalf("Validate(validConfig()) = %v, want nil", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Validate(validConfig()) warnings = %v, want none", warnings)
	}
}

// TestValidateRules is one golden error per documented rule: mutate
// validConfig() to break exactly one rule, and require the error text
// names it.
func TestValidateRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "rule12_base_branch_is_an_option",
			mutate: func(c *Config) {
				c.Host.DefaultBaseBranch = "--upload-pack=touch /tmp/x"
			},
			wantErr: `host.default_base_branch: "--upload-pack=touch /tmp/x" is not a valid git branch or ref name: it starts with "-"`,
		},
		{
			name: "rule12_base_branch_empty",
			mutate: func(c *Config) {
				c.Host.DefaultBaseBranch = ""
			},
			wantErr: `host.default_base_branch: "" is not a valid git branch or ref name: it is empty`,
		},
		{
			name: "rule1_team_not_defined",
			mutate: func(c *Config) {
				c.Routing.ProjectKeyMap["PROJ"] = ProjectKeyMapEntry{Team: "Ghost Team", Repos: []string{"acme.billing.api"}}
			},
			wantErr: `team "Ghost Team" is not defined`,
		},
		{
			name: "rule2_missing_tier",
			mutate: func(c *Config) {
				c.Ops.Environments["dev"] = Environment{}
			},
			wantErr: "ops.environments.dev: missing tier",
		},
		{
			name: "rule2_invalid_tier",
			mutate: func(c *Config) {
				c.Ops.Environments["dev"] = Environment{Tier: "staging"}
			},
			wantErr: `tier "staging" is not one of sandbox, nonprod, prod`,
		},
		{
			name: "rule3_target_map_bad_shape",
			mutate: func(c *Config) {
				c.Ops.TargetMap = map[string]string{"no-colon-here": "prod"}
			},
			wantErr: `key "no-colon-here" is not shaped <repo>:<path>`,
		},
		{
			name: "rule3_target_map_unconfigured_repo",
			mutate: func(c *Config) {
				c.Ops.TargetMap = map[string]string{"unknown.repo:infra/prod": "prod"}
			},
			wantErr: `names repository "unknown.repo", which is not configured`,
		},
		{
			name: "rule4_risk_gate_too_high",
			mutate: func(c *Config) {
				c.Ops.RiskGate = 101
			},
			wantErr: "ops.risk_gate: 101 is outside [0, 100]",
		},
		{
			name: "rule4_risk_gate_negative",
			mutate: func(c *Config) {
				c.Ops.RiskGate = -1
			},
			wantErr: "ops.risk_gate: -1 is outside [0, 100]",
		},
		{
			name: "rule5_default_environment_unconfigured",
			mutate: func(c *Config) {
				c.Ops.DefaultEnvironment = "does-not-exist"
			},
			wantErr: `ops.default_environment: "does-not-exist" is not defined`,
		},
		{
			name: "rule6_repo_settings_unconfigured_repo",
			mutate: func(c *Config) {
				c.Routing.Teams["Team Beta"] = Team{
					RepoSettings: map[string]RepoSettings{"unknown.repo": {MutationThreshold: 90}},
				}
			},
			wantErr: "routing.teams.Team Beta.repo_settings.unknown.repo: repository is not configured",
		},
		{
			name: "rule7_tier_multiplier_too_precise",
			mutate: func(c *Config) {
				c.Ops.Risk.TierMultiplier.Sandbox = 0.23456
			},
			wantErr: "ops.risk.tier_multiplier.sandbox",
		},
		{
			name: "rule9_e2e_sandbox_env_missing",
			mutate: func(c *Config) {
				c.Ops.E2E = E2E{Enabled: true, SandboxEnvironment: "does-not-exist"}
			},
			wantErr: `ops.e2e.sandbox_environment: "does-not-exist" is not defined`,
		},
		{
			name: "rule9_e2e_sandbox_env_wrong_tier",
			mutate: func(c *Config) {
				c.Ops.E2E = E2E{Enabled: true, SandboxEnvironment: "dev"}
			},
			wantErr: `must be sandbox-tier when ops.e2e.enabled, has tier "nonprod"`,
		},
		{
			name: "rule10_standards_source_missing_fields",
			mutate: func(c *Config) {
				c.Standards.Sources = append(c.Standards.Sources, StandardsSource{ID: "incomplete"})
			},
			wantErr: "standards source must have id, repo and ref",
		},
		{
			name: "rule10_standards_source_duplicate_id",
			mutate: func(c *Config) {
				c.Routing.Teams["Team Beta"] = Team{
					Standards: []StandardsSource{{ID: "acme-org", Repo: "https://example.com/x", Ref: "v1"}},
				}
			},
			wantErr: `standards source id "acme-org" is already used at standards.sources[0]`,
		},
		{
			name: "rule11_schema_version",
			mutate: func(c *Config) {
				c.SchemaVersion = 2
			},
			wantErr: "schema_version: 2 is not supported",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(cfg)
			_, err := Validate(cfg, "")
			if err == nil {
				t.Fatalf("Validate: want an error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestValidateAggregatesEveryViolation proves errors.Join, not
// first-hit: two independent violations both appear in one error.
func TestValidateAggregatesEveryViolation(t *testing.T) {
	cfg := validConfig()
	cfg.Ops.RiskGate = 200
	cfg.Ops.DefaultEnvironment = "nowhere"

	_, err := Validate(cfg, "")
	if err == nil {
		t.Fatal("Validate: want an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ops.risk_gate") {
		t.Errorf("aggregated error missing the risk_gate violation: %q", msg)
	}
	if !strings.Contains(msg, "ops.default_environment") {
		t.Errorf("aggregated error missing the default_environment violation: %q", msg)
	}
}

func TestHasAtMostThreeDecimals(t *testing.T) {
	cases := []struct {
		v    float64
		want bool
	}{
		{0.2, true},
		{1, true},
		{3, true},
		{0.001, true},
		{0.123, true},
		{0.1234, false},
		{0.23456, false},
	}
	for _, c := range cases {
		if got := hasAtMostThreeDecimals(c.v); got != c.want {
			t.Errorf("hasAtMostThreeDecimals(%v) = %v, want %v", c.v, got, c.want)
		}
	}
}
