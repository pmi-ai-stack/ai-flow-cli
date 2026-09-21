package config

import (
	"encoding/json"
	"testing"
	"time"
)

// validConfig returns a Config that satisfies every Validate rule —
// the baseline every rule-specific test in validate_test.go mutates to
// break exactly one rule. It deliberately does not equal the embedded
// template (which is intentionally near-empty); this one exercises
// every non-empty block Validate has a rule for.
func validConfig() *Config {
	return &Config{
		SchemaVersion: CurrentSchemaVersion,
		Tracker: Tracker{
			Provider: "jira",
			BaseURL:  "https://acme.atlassian.net",
			Statuses: TrackerStatuses{
				Todo:       []string{"To Do"},
				InProgress: "In Progress",
				InReview:   "In Review",
			},
			BugTypes:     []string{"Bug"},
			CommentLimit: 5,
		},
		Host: Host{Provider: "github", DefaultBaseBranch: "main"},
		Routing: Routing{
			ProjectKeyMap: map[string]ProjectKeyMapEntry{
				"PROJ": {Team: "Team Beta", Repos: []string{"acme.billing.api"}},
			},
			Teams: map[string]Team{
				"Team Beta": {
					LocalBase: "~/projects",
					RepoSettings: map[string]RepoSettings{
						"acme.billing.api": {MutationThreshold: 90},
					},
					Standards: []StandardsSource{
						{ID: "team-beta", Repo: "https://github.com/acme/team-beta-standards", Ref: "v1.0.0"},
					},
				},
			},
		},
		Defaults: Defaults{
			MutationThreshold:      80,
			MutationMaxRounds:      3,
			MutationTimeoutMinutes: 20,
			TestTimeoutMinutes:     20,
			GuardIdleHours:         8,
		},
		Standards: Standards{
			Sources: []StandardsSource{
				{ID: "acme-org", Repo: "https://github.com/acme/standards", Ref: "v3.2.0"},
			},
			GuidanceBudgetTokens: 6000,
		},
		Ops: Ops{
			Environments: map[string]Environment{
				"dev":     {Tier: "nonprod"},
				"prod":    {Tier: "prod"},
				"sandbox": {Tier: "sandbox"},
			},
			TargetMap:          map[string]string{"acme.billing.api:infra/prod": "prod"},
			DefaultEnvironment: "dev",
			RiskGate:           40,
			Risk: Risk{
				ActionWeight:   ActionWeight{Create: 1, Update: 2, Replace: 5, Delete: 8},
				KindWeight:     KindWeight{Workload: 2, Network: 3, RBAC: 4, Secret: 4, Data: 6, IAM: 6, CRDOrNamespace: 5, Pipeline: 2, Other: 1},
				TierMultiplier: TierMultiplier{Sandbox: 0.2, Nonprod: 1, Prod: 3},
				DriftPenalty:   5,
				OfflinePenalty: 10,
			},
			E2E:                E2E{Enabled: true, SandboxEnvironment: "sandbox"},
			ToolTimeoutMinutes: 30,
		},
	}
}

// TestConfigRoundTripsAllFields marshal/unmarshals a fully-populated and
// a minimal Config and checks both survive the round trip unchanged —
// 10-data-types.md's prescribed test for a struct whose json tags must
// match the schema field for field.
func TestConfigRoundTripsAllFields(t *testing.T) {
	for name, cfg := range map[string]*Config{
		"full":    validConfig(),
		"minimal": {SchemaVersion: 1},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			got, err := decode(data)
			if err != nil {
				t.Fatalf("decode(Marshal(cfg)): %v", err)
			}
			gotBytes, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal(decoded): %v", err)
			}
			if string(gotBytes) != string(data) {
				t.Errorf("round trip changed bytes:\n--- before ---\n%s\n--- after ---\n%s", data, gotBytes)
			}
		})
	}
}

// TestDurationAccessors proves call sites never re-parse a minutes/hours
// literal themselves (10-data-types.md): each accessor converts the
// stored integer into the time.Duration it means.
func TestDurationAccessors(t *testing.T) {
	d := Defaults{MutationTimeoutMinutes: 20, TestTimeoutMinutes: 15, GuardIdleHours: 8}
	if got, want := d.MutationTimeout(), 20*time.Minute; got != want {
		t.Errorf("MutationTimeout() = %v, want %v", got, want)
	}
	if got, want := d.TestTimeout(), 15*time.Minute; got != want {
		t.Errorf("TestTimeout() = %v, want %v", got, want)
	}
	if got, want := d.GuardIdleDuration(), 8*time.Hour; got != want {
		t.Errorf("GuardIdleDuration() = %v, want %v", got, want)
	}

	o := Ops{ToolTimeoutMinutes: 30}
	if got, want := o.ToolTimeout(), 30*time.Minute; got != want {
		t.Errorf("ToolTimeout() = %v, want %v", got, want)
	}
}
