package config

import "testing"

func TestResolveMutationThreshold(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{MutationThreshold: 80},
		Routing: Routing{
			Teams: map[string]Team{
				"Team Beta": {RepoSettings: map[string]RepoSettings{
					"acme.billing.api": {MutationThreshold: 90},
				}},
			},
		},
	}

	cases := []struct {
		name string
		team string
		repo string
		want int
	}{
		{"override present", "Team Beta", "acme.billing.api", 90},
		{"repo not overridden", "Team Beta", "acme.billing.web", 80},
		{"team not configured", "Unknown Team", "acme.billing.api", 80},
		{"zero override means unset", "Team Zero", "acme.zero", 80},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveMutationThreshold(cfg, c.team, c.repo); got != c.want {
				t.Errorf("ResolveMutationThreshold(%q, %q) = %d, want %d", c.team, c.repo, got, c.want)
			}
		})
	}
}

func TestResolveMutationThresholdZeroOverrideFallsThrough(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{MutationThreshold: 80},
		Routing: Routing{
			Teams: map[string]Team{
				"Team Beta": {RepoSettings: map[string]RepoSettings{
					"acme.billing.api": {MutationThreshold: 0},
				}},
			},
		},
	}
	if got := ResolveMutationThreshold(cfg, "Team Beta", "acme.billing.api"); got != 80 {
		t.Errorf("ResolveMutationThreshold with zero override = %d, want 80 (fall through to defaults)", got)
	}
}
