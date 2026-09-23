package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// stageResult is the result shape every mutating `stage` subcommand
// returns.
type stageResult struct {
	Key   string `json:"key"`
	Stage string `json:"stage"`
	State string `json:"state"`
}

// shouldRunResult is `stage should-run`'s result shape.
type shouldRunResult struct {
	Stage string `json:"stage"`
	Rule  string `json:"rule"`
	Run   bool   `json:"run"`
}

func newStageCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stage",
		Short: "Drive a ticket's stage state machine",
	}
	cmd.AddCommand(newStageShouldRunCmd(cio))
	cmd.AddCommand(newStageStartCmd(cio))
	cmd.AddCommand(newStageCompleteCmd(cio))
	cmd.AddCommand(newStageApproveCmd(cio))
	cmd.AddCommand(newStageSkipCmd(cio))
	cmd.AddCommand(newStageResetCmd(cio))
	return cmd
}

// parseKeyStage validates both positionals every `stage` subcommand
// takes: the ticket key (always first, per the spec's "ticket-scoped
// commands" rule) and the stage id.
func parseKeyStage(args []string) (string, handoff.Stage, error) {
	key, stageArg := args[0], args[1]
	if err := handoff.ValidateKey(key); err != nil {
		return "", "", err
	}
	stage := handoff.Stage(stageArg)
	if !handoff.IsStage(stage) {
		return "", "", &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("unknown stage %q", stageArg),
			Details: []string{"one of: " + strings.Join(handoff.StageNames(), ", ")},
		}
	}
	return key, stage, nil
}

// finalizeTransition is the save+record+log sequence every mutating
// stage command performs after a successful transition: write
// status.json, rewrite active.json for this ticket, append one
// stage_transition event.
func finalizeTransition(home, key string, stage handoff.Stage, st *handoff.Status, command string, from, to handoff.State, reason string) error {
	if _, err := handoff.SaveStatus(home, st); err != nil {
		return err
	}
	if err := handoff.RecordActive(home, key, stage); err != nil {
		return err
	}
	return handoff.AppendEvent(handoff.TicketEventsPath(home, key), handoff.StageTransitionEvent(stage, from, to, command, reason))
}

// buildShouldRunInput computes handoff.ShouldRunInput for stage — the
// phase-6 plan's design decision 7, extended by phase 7 for mutation
// and phase 20 for render-drift/dry-run. Every stage but those five
// ignores the input entirely (see handoff.ShouldRun's own switch), so
// this returns the zero value for the rest without doing any detection
// work at all: internal/detect.E2E/Mutation's filesystem probing (for
// visual-test/e2e/mutation, via repo.json) and hasChangedTarget's own
// read (for render-drift/dry-run, via ops-targets.json — a separate
// data source, handled in its own early branch below) are both worth
// skipping for every other stage/command call per ticket that doesn't
// need either.
func buildShouldRunInput(f Flags, key string, stage handoff.Stage) (handoff.ShouldRunInput, error) {
	if stage == handoff.StageRenderDrift || stage == handoff.StageDryRun {
		has, err := hasChangedTarget(f, key)
		if err != nil {
			return handoff.ShouldRunInput{}, err
		}
		return handoff.ShouldRunInput{HasChangedTarget: has}, nil
	}
	if stage != handoff.StageVisualTest && stage != handoff.StageE2E && stage != handoff.StageMutation {
		return handoff.ShouldRunInput{}, nil
	}

	ticketDir := handoff.TicketDir(f.Home, key)
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "repo.json"))
	if err != nil {
		return handoff.ShouldRunInput{}, err
	}
	if data == nil {
		// No repo.json yet — true before `locate` completes, long before
		// either real stage is reachable; the zero value (run: false) is
		// harmless since nothing calls Start/ShouldRun for these two
		// stages that early in practice.
		return handoff.ShouldRunInput{}, nil
	}
	var artifact repoArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return handoff.ShouldRunInput{}, fmt.Errorf("cli: decode repo.json: %w", err)
	}

	in := handoff.ShouldRunInput{
		HasFrontendFiles: detect.Frontend(artifact.InScopeFiles),
		HasAppFiles:      len(detect.AppFiles(artifact.InScopeFiles, nil)) > 0,
	}
	// Only the one per-repo filesystem probe stage actually needs runs —
	// e2e's own rule never reads HasMutationTool, and mutation's never
	// reads HasE2ESuite (handoff.ShouldRun's switch), so running both
	// loops for every call would cost real I/O (os.Stat/os.ReadDir per
	// repository) for a field the caller is about to discard.
	switch stage {
	case handoff.StageE2E:
		for _, r := range artifact.Repositories {
			if _, ok, err := detect.E2E(toDetectRepo(r)); err != nil {
				return handoff.ShouldRunInput{}, err
			} else if ok {
				in.HasE2ESuite = true
				break
			}
		}
	case handoff.StageMutation:
		for _, r := range artifact.Repositories {
			if _, ok, err := detect.Mutation(toDetectRepo(r)); err != nil {
				return handoff.ShouldRunInput{}, err
			} else if ok {
				in.HasMutationTool = true
				break
			}
		}
	}
	return in, nil
}

func newStageShouldRunCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "should-run KEY STAGE",
		Short: "Print whether a stage should run, without changing anything",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("stage should-run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key, stage, err := parseKeyStage(args)
			if err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			in, err := buildShouldRunInput(f, key, stage)
			if err != nil {
				return Outcome{}, err
			}
			run, rule := handoff.ShouldRun(stage, in)
			return Outcome{Result: shouldRunResult{Stage: string(stage), Rule: rule, Run: run}}, nil
		}),
	}
}

func newStageStartCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "start KEY STAGE",
		Short: "Start a stage",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("stage start", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key, stage, err := parseKeyStage(args)
			if err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			st, err := handoff.LoadStatus(f.Home, key)
			if err != nil {
				return Outcome{}, err
			}
			from := st.Entry(stage).State

			in, err := buildShouldRunInput(f, key, stage)
			if err != nil {
				return Outcome{}, err
			}
			if err := handoff.Start(st, stage, in); err != nil {
				return Outcome{}, err
			}
			to := st.Entry(stage).State

			if err := resolveOpsDetectForStart(f, key, stage, to); err != nil {
				return Outcome{NeedsInput: needsInputFor(err, key)}, err
			}

			warnings, err := resolveStandardsForStart(f, key, stage, to)
			if err != nil {
				return Outcome{}, err
			}

			if err := finalizeTransition(f.Home, key, stage, st, "stage start", from, to, ""); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: stageResult{Key: key, Stage: string(stage), State: string(to)}, Warnings: warnings}, nil
		}),
	}
}

// resolveStandardsForStart is `stage start`'s standards integration:
// resolve, rewrite standards.json when the resolution hash moved, and
// write the starting stage's own guidance.md.
//
// It can refuse the transition. A failure here returns before
// finalizeTransition persists anything, so `stage start` fails with
// nothing written — deliberately, since a stage whose skill is required
// to read guidance.md first must not start without one. The refusals
// it can produce are missing_repo_instructions (exit 4), pack_unreachable
// (exit 5) and a config-load failure, all raised by the resolver itself.
//
// standards.json is written only on a hash change — the spec's
// "refreshes guidance.md when the resolution hash moved" — but
// guidance.md is written every time, because guidance is *per stage*:
// starting review after implement must rewrite it even though nothing
// about the resolution moved. The write is atomic and byte-stable, so
// rewriting identical bytes is observationally a no-op.
//
// Skipped entirely before the plan stage: fetch and locate run before
// repo.json exists, and repo.json is where the repo layer, the detected
// languages and the waivers all come from. Also skipped for a stage
// that did not actually start (skipped by its own rule, or parked at
// awaiting_approval by a pre-gate) — there is no skill about to read a
// guidance.md for a stage that is not running.
func resolveStandardsForStart(f Flags, key string, stage handoff.Stage, to handoff.State) ([]string, error) {
	if !handoff.PlanOrLater(stage) || to != handoff.StateRunning || f.Simulate {
		return nil, nil
	}
	// repo.json absent means locate has not completed. The stage machine
	// does not enforce pipeline order, so this is reachable — and a
	// stage start that refused because of it would be a new
	// precondition this phase has no mandate to add. Skip silently: the
	// next start after locate resolves, and `standards resolve` itself
	// still reports missing_artifact when called directly.
	if _, err := os.Stat(handoff.RepoPath(f.Home, key)); err != nil {
		return nil, nil //nolint:nilerr // an absent repo.json is "not resolvable yet", not a failure
	}

	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return nil, err
	}
	e, packs, _, err := resolveAndWrite(f, key, cfg)
	defer standards.ClosePacks(packs)
	if err != nil {
		return nil, err
	}

	if _, err := standards.WriteGuidance(handoff.TicketDir(f.Home, key), key, string(stage), e); err != nil {
		return nil, err
	}
	return e.Warnings(), nil
}

// resolveOpsDetectForStart runs `ops detect` at scope "repo" when
// `plan` is the stage actually starting — the spec's own "at plan the
// skill therefore sees every target in the repository." Skipped under
// the same conditions resolveStandardsForStart already documents:
// repo.json absent, the stage not actually starting, or --simulate.
func resolveOpsDetectForStart(f Flags, key string, stage handoff.Stage, to handoff.State) error {
	if stage != handoff.StagePlan || to != handoff.StateRunning || f.Simulate {
		return nil
	}
	ticketDir := handoff.TicketDir(f.Home, key)
	if _, err := os.Stat(handoff.RepoPath(f.Home, key)); err != nil {
		return nil //nolint:nilerr // an absent repo.json is "not resolvable yet", not a failure
	}

	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return err
	}
	artifact, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return err
	}
	result, err := runOpsDetect(f, cfg, artifact, ops.ScopeRepo)
	if err != nil {
		return err
	}
	_, err = handoff.WriteArtifact(ticketDir, "ops-targets.json", "ops-targets", result)
	return err
}

// resolveOpsDetectForComplete runs `ops detect` at scope "diff" when
// `implement` completes successfully — the spec's own "at dry-run the
// targets are diff-scoped." Skipped on the --fail path (nothing new to
// report) and for every other stage.
func resolveOpsDetectForComplete(f Flags, key string, stage handoff.Stage, failFlag bool) error {
	if stage != handoff.StageImplement || failFlag {
		return nil
	}
	ticketDir := handoff.TicketDir(f.Home, key)
	if _, err := os.Stat(handoff.RepoPath(f.Home, key)); err != nil {
		return nil //nolint:nilerr // the stage machine does not enforce pipeline order; an absent repo.json means nothing to detect against yet
	}

	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return err
	}
	artifact, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return err
	}
	result, err := runOpsDetect(f, cfg, artifact, ops.ScopeDiff)
	if err != nil {
		return err
	}
	_, err = handoff.WriteArtifact(ticketDir, "ops-targets.json", "ops-targets", result)
	return err
}

// checkDryRunFreshness enforces "stage complete for impact and
// rollback refuses while any in-scope target's tree hash is newer than
// dryrun-results.json" (Re-running a passed stage) — called from
// `stage complete KEY impact`/`stage complete KEY rollback`, the same
// stage-transition-gating role resolveOpsDetectForStart/
// resolveOpsDetectForComplete already play for start/complete
// respectively. It recomputes each changed target's *current* branch
// tree hash from the live checkout (not the render cache, which could
// itself be stale relative to an edit `plan-rollback` just made — the
// phase-20 plan's design decision 6) and compares it against the hash
// dryrun-results.json recorded for that target, refusing with
// stage_not_allowed (the spec's own documented catch-all for a stage
// precondition the exit-code table does not separately name) on any
// mismatch.
//
// Tolerant of ops-targets.json or dryrun-results.json being entirely
// absent — nothing to compare against yet, not this function's job to
// require dry-run to have run (checkPrimaryArtifact already requires
// impact/rollback's own primary artifact to exist, which in production
// today it never does before phase 23 — see the plan's own §0.10).
func checkDryRunFreshness(f Flags, key string) error {
	ticketDir := handoff.TicketDir(f.Home, key)

	targetsArt, err := loadOpsTargetsTolerant(ticketDir)
	if err != nil || targetsArt == nil {
		return err
	}

	dryRunData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "dryrun-results.json"))
	if err != nil {
		return err
	}
	if dryRunData == nil {
		return nil
	}
	var dryRunArt opsDryRunArtifact
	if err := json.Unmarshal(dryRunData, &dryRunArt); err != nil {
		return fmt.Errorf("cli: decode dryrun-results.json: %w", err)
	}
	hashes := make(map[string]string, len(dryRunArt.Targets))
	for _, t := range dryRunArt.Targets {
		hashes[t.Target] = t.Hash
	}

	repoArt, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return err
	}

	for _, t := range targetsArt.Targets {
		if t.Changed == nil || !*t.Changed {
			continue
		}
		sourceDir, err := resolveTargetSourceDir(f, key, repoArt, t, ops.RefBranch)
		if err != nil {
			return err
		}
		hash, err := render.TreeHash(sourceDir)
		if err != nil {
			return err
		}
		if hashes[ops.TargetKey(t)] != hash {
			return &envelope.Error{
				Code:    "stage_not_allowed",
				Message: fmt.Sprintf("target %s changed since dry-run last ran; run ops dry-run %s first", ops.TargetKey(t), key),
			}
		}
	}
	return nil
}

func newStageCompleteCmd(cio cmdIO) *cobra.Command {
	var failFlag bool
	cmd := &cobra.Command{
		Use:   "complete KEY STAGE",
		Short: "Complete a running stage",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("stage complete", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key, stage, err := parseKeyStage(args)
			if err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			st, err := handoff.LoadStatus(f.Home, key)
			if err != nil {
				return Outcome{}, err
			}
			from := st.Entry(stage).State
			ticketDir := handoff.TicketDir(f.Home, key)

			var gi handoff.GateInputs
			if from == handoff.StateRunning && !failFlag {
				artifactData, err := checkPrimaryArtifact(ticketDir, stage)
				if err != nil {
					return Outcome{}, err
				}
				if stage == handoff.StageChecks {
					open, err := handoff.MustFindingsOpen(ticketDir)
					if err != nil {
						return Outcome{}, err
					}
					if open {
						return Outcome{}, mustFindingsOpenError(stage)
					}
				}
				if stage == handoff.StageImpact {
					cfg, err := config.Load(f.Home, f.Config)
					if err != nil {
						return Outcome{}, err
					}
					score, protected, err := parseImpactGateInputs(artifactData)
					if err != nil {
						return Outcome{}, err
					}
					gi = handoff.GateInputs{RiskGate: cfg.Ops.RiskGate, Score: score, Protected: protected}
				}
				if stage == handoff.StageImpact || stage == handoff.StageRollback {
					if err := checkDryRunFreshness(f, key); err != nil {
						return Outcome{}, err
					}
				}
			}

			if err := handoff.Complete(st, stage, failFlag, gi); err != nil {
				return Outcome{}, err
			}
			to := st.Entry(stage).State

			if err := resolveOpsDetectForComplete(f, key, stage, failFlag); err != nil {
				return Outcome{NeedsInput: needsInputFor(err, key)}, err
			}

			if err := finalizeTransition(f.Home, key, stage, st, "stage complete", from, to, ""); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: stageResult{Key: key, Stage: string(stage), State: string(to)}}, nil
		}),
	}
	cmd.Flags().BoolVar(&failFlag, "fail", false, "mark the stage failed instead of completing it")
	return cmd
}

// checkPrimaryArtifact enforces design decision 5: a stage's one
// headline artifact must exist, and — when it is one of the 17
// schema-registered artifacts — must validate. It returns the file's
// bytes so a caller that needs to read the same artifact again (the
// impact stage's gate inputs) can reuse them instead of a second
// ReadArtifact call.
func checkPrimaryArtifact(ticketDir string, stage handoff.Stage) ([]byte, error) {
	filename := handoff.PrimaryArtifact(stage)
	path := filepath.Join(ticketDir, filename)
	data, err := handoff.ReadArtifact(path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, &envelope.Error{
			Code:    "missing_artifact",
			Message: fmt.Sprintf("%s not found; %s has not produced it yet", filename, stage),
		}
	}
	if artifact, ok := handoff.SchemaArtifactFor(filename); ok {
		if err := schema.Validate(artifact, data); err != nil {
			return nil, &envelope.Error{
				Code:    "invalid_result",
				Message: fmt.Sprintf("%s does not satisfy its own %s schema: %v", filename, artifact, err),
			}
		}
	}
	return data, nil
}

func mustFindingsOpenError(stage handoff.Stage) *envelope.Error {
	return &envelope.Error{
		Code:    "must_findings_open",
		Message: fmt.Sprintf("%s: checks-results.json has an open must finding", stage),
	}
}

// impactGateFields is the subset of impact-results.json GateFires
// needs. checkPrimaryArtifact has already proven the file exists and
// satisfies its schema by the time this is called.
type impactGateFields struct {
	Score     int  `json:"score"`
	Protected bool `json:"protected"`
}

// parseImpactGateInputs decodes impact-results.json's score/protected
// fields from data, which the caller has already read via
// checkPrimaryArtifact — this never re-reads the file.
func parseImpactGateInputs(data []byte) (score int, protected bool, err error) {
	var f impactGateFields
	if err := json.Unmarshal(data, &f); err != nil {
		return 0, false, fmt.Errorf("stage complete impact: decode impact-results.json: %w", err)
	}
	return f.Score, f.Protected, nil
}

func newStageApproveCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "approve KEY STAGE",
		Short: "Approve a stage awaiting approval",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("stage approve", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key, stage, err := parseKeyStage(args)
			if err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			st, err := handoff.LoadStatus(f.Home, key)
			if err != nil {
				return Outcome{}, err
			}
			from := st.Entry(stage).State

			if from == handoff.StateAwaitingApproval && stage == handoff.StageShip {
				if handoff.OutputNotPassed(st) {
					return Outcome{}, &envelope.Error{
						Code:    "output_not_passed",
						Message: "ship cannot be approved until output has passed",
					}
				}
				open, err := handoff.MustFindingsOpen(handoff.TicketDir(f.Home, key))
				if err != nil {
					return Outcome{}, err
				}
				if open {
					return Outcome{}, mustFindingsOpenError(stage)
				}
			}

			by := currentUser()
			at := handoff.Now().UTC().Format(time.RFC3339)
			if err := handoff.Approve(st, stage, by, at); err != nil {
				return Outcome{}, err
			}
			to := st.Entry(stage).State

			if err := finalizeTransition(f.Home, key, stage, st, "stage approve", from, to, ""); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: stageResult{Key: key, Stage: string(stage), State: string(to)}}, nil
		}),
	}
}

// currentUser is stage approve's source for "by" — the spec records
// {by, at, gate} on every approval but documents no --by flag, and
// ai-flow runs on the developer's own machine with no multi-user auth
// model, so the OS identity is the only signal available.
func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	if v := os.Getenv("USERNAME"); v != "" {
		return v
	}
	return "unknown"
}

func newStageSkipCmd(cio cmdIO) *cobra.Command {
	var reasonFlag string
	cmd := &cobra.Command{
		Use:   "skip KEY STAGE",
		Short: "Skip a pending stage",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("stage skip", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key, stage, err := parseKeyStage(args)
			if err != nil {
				return Outcome{}, err
			}
			if reasonFlag == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--reason is required"}
			}
			f := FlagsFrom(cmd.Context())

			st, err := handoff.LoadStatus(f.Home, key)
			if err != nil {
				return Outcome{}, err
			}
			from := st.Entry(stage).State

			if err := handoff.Skip(st, stage); err != nil {
				return Outcome{}, err
			}
			to := st.Entry(stage).State

			if err := finalizeTransition(f.Home, key, stage, st, "stage skip", from, to, reasonFlag); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: stageResult{Key: key, Stage: string(stage), State: string(to)}}, nil
		}),
	}
	cmd.Flags().StringVar(&reasonFlag, "reason", "", "why this stage is being skipped (required)")
	return cmd
}

func newStageResetCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "reset KEY STAGE",
		Short: "Reset a finished stage and every later stage to pending",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("stage reset", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key, stage, err := parseKeyStage(args)
			if err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			st, err := handoff.LoadStatus(f.Home, key)
			if err != nil {
				return Outcome{}, err
			}
			from := st.Entry(stage).State

			if err := handoff.Reset(st, stage); err != nil {
				return Outcome{}, err
			}
			to := st.Entry(stage).State

			if handoff.ImplementOrLater(stage) {
				ticketDir := handoff.TicketDir(f.Home, key)
				if err := os.RemoveAll(filepath.Join(ticketDir, "render")); err != nil {
					return Outcome{}, fmt.Errorf("stage reset: remove render/: %w", err)
				}
				if err := os.RemoveAll(filepath.Join(ticketDir, "plans")); err != nil {
					return Outcome{}, fmt.Errorf("stage reset: remove plans/: %w", err)
				}
			}

			if err := finalizeTransition(f.Home, key, stage, st, "stage reset", from, to, ""); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: stageResult{Key: key, Stage: string(stage), State: string(to)}}, nil
		}),
	}
}
