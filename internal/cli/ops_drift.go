package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

// opsDriftResultTarget is one target's render-drift-results.json entry
// — also this command's own envelope shape, since the two are
// identical (unlike ops dry-run, which keeps "plan" envelope-only).
// ops.DriftReport is embedded anonymously so its own "drift"/
// "drift_resources" fields are promoted to this level, keeping the
// alphabetical-by-JSON-key order (drift, drift_resources, hash,
// target, validation) the phase-20 plan's own §0.3 establishes.
type opsDriftResultTarget struct {
	ops.DriftReport
	Hash       string        `json:"hash"`
	Target     string        `json:"target"`
	Validation []ops.Finding `json:"validation"`
}

// renderDriftArtifact is render-drift-results.json's own top-level
// shape.
type renderDriftArtifact struct {
	Targets []opsDriftResultTarget `json:"targets"`
}

type opsDriftResult struct {
	Key     string                 `json:"key"`
	Targets []opsDriftResultTarget `json:"targets"`
}

func newOpsDriftCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drift KEY",
		Short: "Materialize each changed target from branch and base, validate the branch rendering, and compare base against live",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops drift", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			targetFlag, _ := cmd.Flags().GetString("target")
			f := FlagsFrom(cmd.Context())

			if f.Simulate {
				return Outcome{Result: opsDriftResult{Key: key, Targets: []opsDriftResultTarget{}}}, nil
			}

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			repoArt, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			targetsArt, err := loadOpsTargets(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			targets, err := changedOrOneTarget(targetsArt.Targets, targetFlag)
			if err != nil {
				return Outcome{}, err
			}

			reg := buildOpsRegistry(f, cfg)
			var entries []opsDriftResultTarget
			for _, t := range targets {
				rt, err := driftOneTarget(cmd.Context(), f, key, repoArt, t, reg)
				if err != nil {
					return Outcome{}, err
				}
				if rt != nil {
					entries = append(entries, *rt)
				}
			}
			if entries == nil {
				entries = []opsDriftResultTarget{}
			}

			if _, err := handoff.WriteArtifact(ticketDir, "render-drift-results.json", "render-drift-results", renderDriftArtifact{Targets: entries}); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: opsDriftResult{Key: key, Targets: entries}}, nil
		}),
	}
	cmd.Flags().String("target", "", "operate only on the target named <repo>:<path>")
	return cmd
}

// driftOneTarget materializes t from branch and base, validates the
// branch rendering when the addon supports it, and compares the base
// rendering with live state when the addon supports Drift — degrading
// to drift: offline rather than asking when live access fails (the
// plan's own design decision 1: render-drift never raises
// live_credentials_missing, unlike dry-run, since its own artifact
// already has an "offline" value for exactly this and its own catalog
// row has no --offline flag to convey an answer through). A nil result
// with a nil error means t's addon is not registered in reg — the same
// defensive skip renderOneTarget already uses.
func driftOneTarget(ctx context.Context, f Flags, key string, repoArt *repoArtifact, t ops.Target, reg *ops.Registry) (*opsDriftResultTarget, error) {
	addon, ok := reg.Lookup(t.Addon)
	if !ok {
		return nil, nil
	}

	branchHash, err := renderForTarget(ctx, f, key, repoArt, t, ops.RefBranch, addon)
	if err != nil {
		return nil, err
	}
	baseHash, err := renderForTarget(ctx, f, key, repoArt, t, ops.RefBase, addon)
	if err != nil {
		return nil, err
	}

	var validation []ops.Finding
	if addon.Capabilities().Validate {
		branchRendered, err := render.Load(f.Home, branchHash)
		if err != nil {
			return nil, err
		}
		validation, err = addon.Validate(ctx, branchRendered)
		if err != nil {
			return nil, err
		}
	}
	if validation == nil {
		validation = []ops.Finding{}
	}

	report := ops.DriftReport{Drift: ops.DriftNotApplicable, Resources: []string{}}
	if addon.Capabilities().Drift {
		baseRendered, err := render.Load(f.Home, baseHash)
		if err != nil {
			return nil, err
		}
		got, err := addon.Drift(ctx, t, envFor(t), baseRendered)
		switch {
		case errors.Is(err, ops.ErrLiveCredentialsMissing):
			report = ops.DriftReport{Drift: ops.DriftOffline, Resources: []string{}}
		case err != nil:
			return nil, err
		default:
			report = *got
			if report.Resources == nil {
				report.Resources = []string{}
			}
		}
	}

	return &opsDriftResultTarget{
		DriftReport: report,
		Hash:        branchHash,
		Target:      ops.TargetKey(t),
		Validation:  validation,
	}, nil
}

// loadRenderDriftResults reads ticketDir's render-drift-results.json,
// keyed by target — `ops dry-run`'s own baseline for deciding which
// targets need re-validation (the tree hash moved) and which may reuse
// their prior validation verbatim.
func loadRenderDriftResults(ticketDir string) (map[string]opsDriftResultTarget, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "render-drift-results.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, &envelope.Error{Code: "missing_artifact", Message: "render-drift-results.json not found; run ops drift first"}
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
