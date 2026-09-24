package risk

import (
	"math"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// PerMille converts a config-authored decimal (config.ops.risk's own
// tier_multiplier values, e.g. 0.2) into the integer per-mille value
// the score formula sums as int64 (0.2 -> 200). config.Validate already
// proves every tier_multiplier has at most three decimal places, so
// this conversion is always exact — math.Round only guards against
// float64's own representation noise (0.2*1000 can land at
// 199.99999999999997), never a real rounding decision.
func PerMille(v float64) int64 {
	return int64(math.Round(v * 1000))
}

// Contribution is one action's own recorded contribution — the spec's
// own "contributions[] records the uncapped per-action value in per
// mille." Target/Address/Addon/Action/KindClass/Tier are carried
// through unchanged from the Action that produced it, so a caller can
// render impact-results.json's contributions[] directly from this
// slice with no second lookup. JSON tags live here (not on a separate
// CLI wire type) so internal/cli/ops_impact.go's opsImpactArtifact can
// embed []Contribution directly — mirrors internal/ops/rollback.Result,
// which is likewise both the pure computation's own return type and
// the artifact written to disk. Fields are declared in JSON-key
// alphabetical order (target.go's own documented byte-stability
// convention): action, address, addon, kind_class, per_mille, target,
// tier.
type Contribution struct {
	Action    ops.ActionKind `json:"action"`
	Address   string         `json:"address"`
	Addon     string         `json:"addon"`
	KindClass ops.KindClass  `json:"kind_class"`
	PerMille  int64          `json:"per_mille"`
	Target    string         `json:"target"`
	Tier      string         `json:"tier"`
}

// actionWeightFor reads config.ops.risk.action_weight for a's own
// action kind.
func actionWeightFor(w config.ActionWeight, a ops.ActionKind) int64 {
	switch a {
	case ops.ActionCreate:
		return int64(w.Create)
	case ops.ActionUpdate:
		return int64(w.Update)
	case ops.ActionReplace:
		return int64(w.Replace)
	case ops.ActionDelete:
		return int64(w.Delete)
	case ops.ActionNoop:
		return int64(w.Noop)
	default:
		return 0
	}
}

// kindWeightFor reads config.ops.risk.kind_weight for k. An unknown
// KindClass (never produced by internal/ops.IsKindClass-validated data,
// but defensive here since this function has no error return) weighs
// zero rather than panicking.
func kindWeightFor(w config.KindWeight, k ops.KindClass) int64 {
	switch k {
	case ops.KindWorkload:
		return int64(w.Workload)
	case ops.KindNetwork:
		return int64(w.Network)
	case ops.KindRBAC:
		return int64(w.RBAC)
	case ops.KindSecret:
		return int64(w.Secret)
	case ops.KindData:
		return int64(w.Data)
	case ops.KindIAM:
		return int64(w.IAM)
	case ops.KindCRDOrNamespace:
		return int64(w.CRDOrNamespace)
	case ops.KindPipeline:
		return int64(w.Pipeline)
	case ops.KindOther:
		return int64(w.Other)
	default:
		return 0
	}
}

// tierMultiplierPerMilleFor reads config.ops.risk.tier_multiplier for
// tier, already converted to per mille via PerMille. An unrecognized
// tier (never produced by internal/ops/envs.Bind, which only ever
// names sandbox/nonprod/prod) multiplies by zero rather than guessing.
func tierMultiplierPerMilleFor(m config.TierMultiplier, tier string) int64 {
	switch tier {
	case "sandbox":
		return PerMille(m.Sandbox)
	case "nonprod":
		return PerMille(m.Nonprod)
	case "prod":
		return PerMille(m.Prod)
	default:
		return 0
	}
}

// Contribute computes one action's own contribution: action_weight *
// kind_weight * tier_multiplier_per_mille (the spec's own formula,
// already in per-mille units since tier_multiplier_per_mille is itself
// *1000 of the authored ratio).
func Contribute(a ops.Action, r config.Risk) Contribution {
	pm := actionWeightFor(r.ActionWeight, a.Action) * kindWeightFor(r.KindWeight, a.KindClass) * tierMultiplierPerMilleFor(r.TierMultiplier, a.Tier)
	return Contribution{
		Target: a.Target, Address: a.Address, Addon: a.Addon,
		Action: a.Action, KindClass: a.KindClass, Tier: a.Tier,
		PerMille: pm,
	}
}

// Score computes the spec's own formula:
//
//	score = min(100, round_half_up((Σ contribution(a) + 1000*(drift_penalty*targetsWithDrift + offline_penalty*targetsOffline)) / 1000))
//
// entirely in int64, with one round-half-up at the very end
// ((total+500)/1000, safe since every term here is non-negative by
// construction — config weights and penalties are never authored
// negative, and config.Validate does not need to forbid it since a
// negative score would simply clamp to 0 below, never panic or
// misbehave). actions is expected pre-sorted by the caller
// (target, address) — this function does not sort, so contributions is
// returned in actions' own input order (design decision 2).
func Score(actions []ops.Action, targetsWithDrift, targetsOffline int, r config.Risk) (score int, contributions []Contribution) {
	contributions = make([]Contribution, len(actions))
	var total int64
	for i, a := range actions {
		c := Contribute(a, r)
		contributions[i] = c
		total += c.PerMille
	}
	total += 1000 * int64(r.DriftPenalty) * int64(targetsWithDrift)
	total += 1000 * int64(r.OfflinePenalty) * int64(targetsOffline)

	rounded := (total + 500) / 1000
	switch {
	case rounded < 0:
		rounded = 0
	case rounded > 100:
		rounded = 100
	}
	return int(rounded), contributions
}
