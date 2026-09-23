package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// opsDryRunArtifactTarget is one target's dryrun-results.json entry —
// deliberately narrower than opsDryRunResultTarget (no "plan" path):
// the spec's own field list for this file is "mode... and refreshed
// validation[] per target," nothing about where the changeset itself
// landed (that is plans/<target>.json's own, unschematized concern).
type opsDryRunArtifactTarget struct {
	Hash       string        `json:"hash"`
	Target     string        `json:"target"`
	Validation []ops.Finding `json:"validation"`
}

type opsDryRunArtifact struct {
	Mode    string                    `json:"mode"`
	Targets []opsDryRunArtifactTarget `json:"targets"`
}

// opsDryRunResultTarget is this command's own envelope shape — like
// opsDryRunArtifactTarget plus "plan", the ticket-relative path
// plans/<target>.json was written to, useful in the envelope without
// forcing a second lookup, but never part of the schema-validated
// artifact itself.
type opsDryRunResultTarget struct {
	Hash       string        `json:"hash"`
	Plan       string        `json:"plan"`
	Target     string        `json:"target"`
	Validation []ops.Finding `json:"validation"`
}

type opsDryRunResult struct {
	Key     string                  `json:"key"`
	Mode    string                  `json:"mode"`
	Targets []opsDryRunResultTarget `json:"targets"`
}

func newOpsDryRunCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dry-run KEY",
		Short: "Re-render and re-validate every changed target whose tree hash moved, then compute a read-only Changeset",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops dry-run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			offlineFlag, _ := cmd.Flags().GetBool("offline")
			targetFlag, _ := cmd.Flags().GetString("target")
			f := FlagsFrom(cmd.Context())
			mode := "live"
			if offlineFlag {
				mode = "offline"
			}

			if f.Simulate {
				return Outcome{Result: opsDryRunResult{Key: key, Mode: mode, Targets: []opsDryRunResultTarget{}}}, nil
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
			drift, err := loadRenderDriftResults(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			reg := buildOpsRegistry(f, cfg)
			var results []opsDryRunResultTarget
			var artifactTargets []opsDryRunArtifactTarget
			for _, t := range targets {
				rt, at, err := dryRunOneTarget(cmd.Context(), f, key, repoArt, t, reg, drift, offlineFlag)
				if err != nil {
					return Outcome{NeedsInput: needsInputFor(err, key)}, err
				}
				if rt != nil {
					results = append(results, *rt)
					artifactTargets = append(artifactTargets, *at)
				}
			}
			if results == nil {
				results = []opsDryRunResultTarget{}
			}
			if artifactTargets == nil {
				artifactTargets = []opsDryRunArtifactTarget{}
			}

			artifact := opsDryRunArtifact{Mode: mode, Targets: artifactTargets}
			if _, err := handoff.WriteArtifact(ticketDir, "dryrun-results.json", "dryrun-results", artifact); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: opsDryRunResult{Key: key, Mode: mode, Targets: results}}, nil
		}),
	}
	cmd.Flags().Bool("offline", false, "compute every target's changeset without live access")
	cmd.Flags().String("target", "", "operate only on the target named <repo>:<path>")
	return cmd
}

// dryRunOneTarget re-renders t's branch rendering (cheap — already
// memoized by tree hash), re-validates only when that hash moved since
// render-drift's own recorded one for t (else reuses render-drift's
// validation verbatim — the plan's own "re-render and re-validate
// every target whose tree hash moved" reading), computes t's Changeset
// via Addon.DryRun (the unbound Env when offline is true, t's own
// live-bound Env otherwise — design decision 2: a whole-run decision,
// not per-target), and writes the normalized, redacted changeset to
// plans/<repo>/<addon>/<path>.json. A nil result with a nil error means
// t's addon is not registered in reg — the same defensive skip
// renderOneTarget/driftOneTarget already use. An unbound-forced
// ops.ErrLiveCredentialsMissing (offline was not requested) is
// translated to the documented live_credentials_missing envelope
// error, letting the caller ask "provide or --offline".
func dryRunOneTarget(ctx context.Context, f Flags, key string, repoArt *repoArtifact, t ops.Target, reg *ops.Registry, drift map[string]opsDriftResultTarget, offline bool) (*opsDryRunResultTarget, *opsDryRunArtifactTarget, error) {
	addon, ok := reg.Lookup(t.Addon)
	if !ok {
		return nil, nil, nil
	}

	hash, err := renderForTarget(ctx, f, key, repoArt, t, ops.RefBranch, addon)
	if err != nil {
		return nil, nil, err
	}

	prior, hadPrior := drift[ops.TargetKey(t)]
	needsRevalidate := addon.Capabilities().Validate && (!hadPrior || prior.Hash != hash)

	// Loaded at most once and shared by Validate and DryRun below — both
	// need the same real rendering for the same hash, and render.Load
	// re-reads the cache's own markings sidecar from disk each call, so
	// loading it twice for one target would be a wasted read.
	var rendered *ops.Rendered
	if needsRevalidate || addon.Capabilities().DryRun {
		rendered, err = render.Load(f.Home, hash)
		if err != nil {
			return nil, nil, err
		}
	}

	var validation []ops.Finding
	switch {
	case needsRevalidate:
		validation, err = addon.Validate(ctx, rendered)
		if err != nil {
			return nil, nil, err
		}
	case hadPrior:
		validation = prior.Validation
	}
	if validation == nil {
		validation = []ops.Finding{}
	}

	planPath := ""
	if addon.Capabilities().DryRun {
		env := envFor(t)
		if offline {
			env = offlineEnvFor(t)
		}
		changeset, err := addon.DryRun(ctx, t, env, rendered)
		if errors.Is(err, ops.ErrLiveCredentialsMissing) {
			return nil, nil, &envelope.Error{
				Code:    "live_credentials_missing",
				Message: fmt.Sprintf("target %s: live credentials required to compute a changeset; provide them or retry with --offline", ops.TargetKey(t)),
			}
		}
		if err != nil {
			return nil, nil, err
		}

		normalized := normalizeChangeset(*changeset, rendered.Marked)
		dir, filename := planFilePath(t)
		absDir := filepath.Join(handoff.TicketDir(f.Home, key), "plans", dir)
		if _, err := handoff.WriteRaw(absDir, filename, normalized); err != nil {
			return nil, nil, err
		}
		planPath = filepath.Join("plans", dir, filename)
	}

	return &opsDryRunResultTarget{Hash: hash, Plan: planPath, Target: ops.TargetKey(t), Validation: validation},
		&opsDryRunArtifactTarget{Hash: hash, Target: ops.TargetKey(t), Validation: validation}, nil
}

// planFilePath is plans/<target>.json's real, filesystem-safe location
// — plans/<repo>/<addon>/<path>.json, nested rather than a single
// flattened token, mirroring internal/ops/render's own targetDir (the
// phase-19 plan's design decision 1): "<target>" ("<repo>:<path>") is
// not itself a legal path component, and this avoids the identical
// collision phase 19 already solved for render/ (two addons at the
// same path; two repositories sharing a target path — phase 5's still
// -open monorepo question) without inventing a second escaping
// convention.
func planFilePath(t ops.Target) (dir, filename string) {
	full := filepath.Join(t.Repo, t.Addon, filepath.FromSlash(t.Path)) + ".json"
	return filepath.Dir(full), filepath.Base(full)
}

// normalizeChangeset is plans/<target>.json's own normalization step:
// every FieldsChanged value passed through redact.RedactMarked over
// the branch rendering's own sensitivity markings (Determinism §6:
// "plans/<target>.json is redacted from the same markings"), and every
// Action sorted by Address — the one field the spec's own risk-scoring
// section treats as an action's stable, addon-independent identity
// (design decision 5) — for byte stability regardless of the order an
// addon's own DryRun happened to return them in.
func normalizeChangeset(cs ops.Changeset, marked []redact.MarkedValue) ops.Changeset {
	actions := make([]ops.Action, len(cs.Actions))
	for i, a := range cs.Actions {
		na := a
		fc := make([]string, len(a.FieldsChanged))
		for j, v := range a.FieldsChanged {
			fc[j] = redact.RedactMarked(v, marked)
		}
		na.FieldsChanged = fc
		actions[i] = na
	}
	slices.SortFunc(actions, func(a, b ops.Action) int { return cmp.Compare(a.Address, b.Address) })
	return ops.Changeset{Actions: actions}
}
