package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/risk"
)

// targetChangeset pairs one target with its own already-computed
// Changeset — the join loadTargetChangesets performs between
// dryrun-results.json (the post-dry-run target scope and each target's
// hash) and ops-targets.json (each target's full Addon/Repo/Path,
// needed to locate its own plans/<target>.json — see planFilePath).
// Shared by `ops impact`, `ops rollback` and should-run's
// hasChangesetAction.
type targetChangeset struct {
	Target    ops.Target
	Hash      string
	Changeset ops.Changeset
}

// loadTargetChangesets returns (nil, "", nil) when dryrun-results.json
// has not been written yet — impact/rollback cannot run before
// dry-run, but this stays tolerant of that absence (like
// loadOpsTargetsTolerant) rather than erroring, since should-run's own
// hasChangesetAction must answer false, not fail, before dry-run has
// ever run.
func loadTargetChangesets(ticketDir string) ([]targetChangeset, string, error) {
	dryRunData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "dryrun-results.json"))
	if err != nil {
		return nil, "", err
	}
	if dryRunData == nil {
		return nil, "", nil
	}
	var dryRunArt opsDryRunArtifact
	if err := json.Unmarshal(dryRunData, &dryRunArt); err != nil {
		return nil, "", fmt.Errorf("cli: decode dryrun-results.json: %w", err)
	}

	targetsArt, err := loadOpsTargetsTolerant(ticketDir)
	if err != nil {
		return nil, "", err
	}
	byKey := make(map[string]ops.Target)
	if targetsArt != nil {
		for _, t := range targetsArt.Targets {
			byKey[ops.TargetKey(t)] = t
		}
	}

	out := make([]targetChangeset, 0, len(dryRunArt.Targets))
	for _, dt := range dryRunArt.Targets {
		t, ok := byKey[dt.Target]
		if !ok {
			return nil, "", fmt.Errorf("cli: dryrun-results.json names target %q not present in ops-targets.json", dt.Target)
		}
		cs, err := loadPlanChangeset(ticketDir, t)
		if err != nil {
			return nil, "", err
		}
		out = append(out, targetChangeset{Target: t, Hash: dt.Hash, Changeset: cs})
	}
	return out, dryRunArt.Mode, nil
}

// loadPlanChangeset reads t's own plans/<target>.json (planFilePath's
// own location), returning an empty Changeset — not an error — when
// the file does not exist: a target whose addon has no DryRun
// capability is present in dryrun-results.json with hash/validation
// only, and dryRunOneTarget never writes a plan file for it at all.
func loadPlanChangeset(ticketDir string, t ops.Target) (ops.Changeset, error) {
	dir, filename := planFilePath(t)
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "plans", dir, filename))
	if err != nil {
		return ops.Changeset{}, err
	}
	if data == nil {
		return ops.Changeset{}, nil
	}
	var cs ops.Changeset
	if err := json.Unmarshal(data, &cs); err != nil {
		return ops.Changeset{}, fmt.Errorf("cli: decode plan for %s: %w", ops.TargetKey(t), err)
	}
	return cs, nil
}

// hasChangesetAction reports whether any target's own changeset (via
// loadTargetChangesets, tolerant of dry-run never having run) carries
// at least one non-noop action — `stage should-run`'s own input for
// rollback's "changeset_empty" rule.
func hasChangesetAction(f Flags, key string) (bool, error) {
	tcs, _, err := loadTargetChangesets(handoff.TicketDir(f.Home, key))
	if err != nil {
		return false, err
	}
	for _, tc := range tcs {
		for _, a := range tc.Changeset.Actions {
			if a.Action != ops.ActionNoop {
				return true, nil
			}
		}
	}
	return false, nil
}

// checkNoProtectedActionForSkip implements "stage skip KEY impact is
// refused when any action is protected" — the one gate trigger skip
// itself is refused for (the spec's own words). Evaluated directly
// against the already-computed changeset via the same pure
// risk.ActionProtected formula ops impact itself uses, never against
// impact-results.json: skip is only ever legal from pending, i.e.
// before ops impact has had any chance to run. Tolerant of no
// dry-run having run yet (loadTargetChangesets returns nil) — nothing
// to refuse on.
func checkNoProtectedActionForSkip(f Flags, key string) error {
	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return err
	}
	tcs, _, err := loadTargetChangesets(handoff.TicketDir(f.Home, key))
	if err != nil {
		return err
	}
	for _, tc := range tcs {
		for _, a := range tc.Changeset.Actions {
			if risk.ActionProtected(a, cfg.Ops.ProtectedKinds) {
				return &envelope.Error{
					Code:    "stage_not_allowed",
					Message: fmt.Sprintf("stage skip impact: action %s (%s) is protected; impact cannot be skipped", a.Address, a.Target),
				}
			}
		}
	}
	return nil
}

// opsImpactArtifact is impact-results.json's own top-level shape —
// field order alphabetical by JSON key (blast_radius, contributions,
// protected_actions, score, targets_offline, targets_with_drift).
// Contributions/BlastRadius embed internal/ops/risk's own JSON-tagged
// types directly rather than a separate CLI wire type — risk.Score/
// risk.ComputeBlastRadius already return exactly this artifact shape,
// mirroring internal/ops/rollback.Result's identical "pure computation
// type doubles as the artifact" convention.
type opsImpactArtifact struct {
	BlastRadius      risk.BlastRadius    `json:"blast_radius"`
	Contributions    []risk.Contribution `json:"contributions"`
	ProtectedActions []ops.Action        `json:"protected_actions"`
	Score            int                 `json:"score"`
	TargetsOffline   int                 `json:"targets_offline"`
	TargetsWithDrift int                 `json:"targets_with_drift"`
}

// opsImpactResult is this command's own envelope shape — the artifact
// plus the ticket key, so the printed result is useful without opening
// the file it just wrote.
type opsImpactResult struct {
	Key string `json:"key"`
	opsImpactArtifact
}

func newOpsImpactCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "impact KEY",
		Short: "Score risk and compute blast radius from the already-computed changeset, drift and config.ops.risk",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops impact", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			if f.Simulate {
				return Outcome{Result: opsImpactResult{Key: key, opsImpactArtifact: emptyImpactArtifact()}}, nil
			}

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)

			tcs, mode, err := loadTargetChangesets(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			drift, err := loadRenderDriftResultsTolerant(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			artifact := computeImpact(tcs, drift, mode, &cfg.Ops)

			if _, err := handoff.WriteArtifact(ticketDir, "impact-results.json", "impact-results", artifact); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: opsImpactResult{Key: key, opsImpactArtifact: artifact}}, nil
		}),
	}
}

func emptyImpactArtifact() opsImpactArtifact {
	return opsImpactArtifact{
		BlastRadius:      risk.BlastRadius{Entries: []risk.BlastRadiusEntry{}, KindClassCounts: map[ops.KindClass]int{}},
		Contributions:    []risk.Contribution{},
		ProtectedActions: []ops.Action{},
	}
}

// computeImpact is ops impact's own pure core: sort every target's
// actions into one (target, address)-ordered slice, count drifted/
// offline targets, apply the two flag formulas to every action, and
// call internal/ops/risk for the score, contributions and blast
// radius. Split out from the command body so a test can drive it
// directly against hand-built input, with no filesystem.
func computeImpact(tcs []targetChangeset, drift map[string]opsDriftResultTarget, mode string, opsCfg *config.Ops) opsImpactArtifact {
	var actions []ops.Action
	targetsWithDrift, targetsOffline := 0, 0
	offlineTargets := make(map[string]bool)

	for _, tc := range tcs {
		key := ops.TargetKey(tc.Target)
		if d, ok := drift[key]; ok {
			switch d.Drift {
			case ops.DriftPresent:
				targetsWithDrift++
			case ops.DriftOffline:
				offlineTargets[key] = true
			}
		}
		if mode == "offline" {
			offlineTargets[key] = true
		}
		for _, a := range tc.Changeset.Actions {
			a.Protected = risk.ActionProtected(a, opsCfg.ProtectedKinds)
			a.Sensitive = risk.ActionSensitive(a)
			actions = append(actions, a)
		}
	}
	targetsOffline = len(offlineTargets)

	slices.SortFunc(actions, func(a, b ops.Action) int {
		return cmp.Or(cmp.Compare(a.Target, b.Target), cmp.Compare(a.Address, b.Address))
	})

	score, contributions := risk.Score(actions, targetsWithDrift, targetsOffline, opsCfg.Risk)

	artifact := opsImpactArtifact{
		Contributions:    contributions,
		Score:            score,
		TargetsOffline:   targetsOffline,
		TargetsWithDrift: targetsWithDrift,
		BlastRadius:      risk.ComputeBlastRadius(actions, opsCfg),
		ProtectedActions: []ops.Action{},
	}
	for _, a := range actions {
		if a.Protected {
			artifact.ProtectedActions = append(artifact.ProtectedActions, a)
		}
	}
	return artifact
}

// loadRenderDriftResultsTolerant is loadRenderDriftResults without the
// missing_artifact error — (nil, nil) when render-drift-results.json
// has not been written yet, for ops impact's own drift/offline counts,
// which default to zero rather than refusing.
func loadRenderDriftResultsTolerant(ticketDir string) (map[string]opsDriftResultTarget, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "render-drift-results.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var artifact renderDriftArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("cli: decode render-drift-results.json: %w", err)
	}
	out := make(map[string]opsDriftResultTarget, len(artifact.Targets))
	for _, t := range artifact.Targets {
		out[t.Target] = t
	}
	return out, nil
}
