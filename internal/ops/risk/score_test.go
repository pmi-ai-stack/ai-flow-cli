package risk

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// specExampleRisk mirrors the spec's own config.json example verbatim
// (§ Config schema), so a hand-computed expectation here is checkable
// against the spec's own numbers, not just this package's own formula
// echoed back at itself.
var specExampleRisk = config.Risk{
	ActionWeight:   config.ActionWeight{Create: 1, Update: 2, Replace: 5, Delete: 8, Noop: 0},
	KindWeight:     config.KindWeight{Workload: 2, Network: 3, RBAC: 4, Secret: 4, Data: 6, IAM: 6, CRDOrNamespace: 5, Pipeline: 2, Other: 1},
	TierMultiplier: config.TierMultiplier{Sandbox: 0.2, Nonprod: 1, Prod: 3},
	DriftPenalty:   5,
	OfflinePenalty: 10,
}

func TestPerMille(t *testing.T) {
	cases := []struct {
		v    float64
		want int64
	}{
		{0.2, 200},
		{1, 1000},
		{3, 3000},
		{0.001, 1},
	}
	for _, c := range cases {
		if got := PerMille(c.v); got != c.want {
			t.Errorf("PerMille(%v) = %d, want %d", c.v, got, c.want)
		}
	}
}

func TestContributeUsesSpecFormulaExactly(t *testing.T) {
	a := ops.Action{Action: ops.ActionUpdate, KindClass: ops.KindWorkload, Tier: "prod"}
	c := Contribute(a, specExampleRisk)
	// action_weight[update]=2 * kind_weight[workload]=2 * tier_multiplier_per_mille[prod]=3000 = 12000.
	if c.PerMille != 12000 {
		t.Errorf("PerMille = %d, want 12000", c.PerMille)
	}
}

func TestScoreCapsAt100(t *testing.T) {
	actions := make([]ops.Action, 20)
	for i := range actions {
		actions[i] = ops.Action{Action: ops.ActionDelete, KindClass: ops.KindIAM, Tier: "prod"}
	}
	score, contributions := Score(actions, 0, 0, specExampleRisk)
	if score != 100 {
		t.Errorf("score = %d, want 100 (capped)", score)
	}
	if len(contributions) != 20 {
		t.Errorf("len(contributions) = %d, want 20 (uncapped, one per action)", len(contributions))
	}
	for _, c := range contributions {
		if c.PerMille <= 100000 {
			t.Errorf("contribution PerMille = %d, want > 100000 (uncapped)", c.PerMille)
		}
	}
}

func TestScoreDriftAndOfflinePenaltiesAddFullPoints(t *testing.T) {
	// No actions at all: the penalty terms alone should produce
	// drift_penalty*targetsWithDrift + offline_penalty*targetsOffline
	// score points, per "adds offline_penalty to the score."
	score, _ := Score(nil, 2, 1, specExampleRisk)
	want := 5*2 + 10*1 // 20
	if score != want {
		t.Errorf("score = %d, want %d", score, want)
	}
}

// TestScoreRoundHalfUpBoundary proves the exact round-half-up rule
// with a config crafted to land contributions on a precise per-mille
// boundary, independent of the spec's own example weights.
func TestScoreRoundHalfUpBoundary(t *testing.T) {
	boundaryRisk := config.Risk{
		ActionWeight:   config.ActionWeight{Update: 1},
		KindWeight:     config.KindWeight{Workload: 1},
		TierMultiplier: config.TierMultiplier{Prod: 0.5}, // 500 per mille
	}
	action := ops.Action{Action: ops.ActionUpdate, KindClass: ops.KindWorkload, Tier: "prod"}

	// Exactly 500 per mille total: round_half_up(0.5) = 1.
	score, contributions := Score([]ops.Action{action}, 0, 0, boundaryRisk)
	if score != 1 {
		t.Errorf("score = %d, want 1 (round half up of exactly 0.5)", score)
	}
	if contributions[0].PerMille != 500 {
		t.Errorf("contribution PerMille = %d, want 500", contributions[0].PerMille)
	}

	// Just under: 499 per mille (weight 499 rather than 500) rounds
	// down to 0.
	justUnder := config.Risk{
		ActionWeight: config.ActionWeight{Update: 1},
		KindWeight:   config.KindWeight{Workload: 1},
		// 0.499 has more than three decimals, which config.Validate
		// would reject in real config content — this test drives
		// Score directly, bypassing config validation on purpose, to
		// isolate the arithmetic boundary from that unrelated rule.
		TierMultiplier: config.TierMultiplier{Prod: 0.499},
	}
	score2, _ := Score([]ops.Action{action}, 0, 0, justUnder)
	if score2 != 0 {
		t.Errorf("score = %d, want 0 (round half up of 0.499 rounds down)", score2)
	}
}

// TestScoreFloatFreeDeterminism repeats an identical computation many
// times and asserts byte-identical results — the plan's own
// "float-free determinism test."
func TestScoreFloatFreeDeterminism(t *testing.T) {
	actions := []ops.Action{
		{Action: ops.ActionUpdate, KindClass: ops.KindWorkload, Tier: "prod"},
		{Action: ops.ActionDelete, KindClass: ops.KindData, Tier: "prod"},
		{Action: ops.ActionCreate, KindClass: ops.KindNetwork, Tier: "nonprod"},
		{Action: ops.ActionReplace, KindClass: ops.KindSecret, Tier: "sandbox"},
	}
	first, firstContrib := Score(actions, 3, 2, specExampleRisk)
	for i := 0; i < 200; i++ {
		got, gotContrib := Score(actions, 3, 2, specExampleRisk)
		if got != first {
			t.Fatalf("run %d: score = %d, want %d (first run)", i, got, first)
		}
		for j := range gotContrib {
			if gotContrib[j] != firstContrib[j] {
				t.Fatalf("run %d: contribution %d = %+v, want %+v", i, j, gotContrib[j], firstContrib[j])
			}
		}
	}
}

func TestScoreNeverNegative(t *testing.T) {
	// noop actions and zero penalties: total is exactly zero.
	score, _ := Score([]ops.Action{{Action: ops.ActionNoop, KindClass: ops.KindOther, Tier: "prod"}}, 0, 0, specExampleRisk)
	if score != 0 {
		t.Errorf("score = %d, want 0", score)
	}
}
