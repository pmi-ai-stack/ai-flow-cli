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
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
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

func newStageShouldRunCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "should-run KEY STAGE",
		Short: "Print whether a stage should run, without changing anything",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("stage should-run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			_, stage, err := parseKeyStage(args)
			if err != nil {
				return Outcome{}, err
			}
			run, rule := handoff.ShouldRun(stage)
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

			if err := handoff.Start(st, stage); err != nil {
				return Outcome{}, err
			}
			to := st.Entry(stage).State

			if err := finalizeTransition(f.Home, key, stage, st, "stage start", from, to, ""); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: stageResult{Key: key, Stage: string(stage), State: string(to)}}, nil
		}),
	}
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
			}

			if err := handoff.Complete(st, stage, failFlag, gi); err != nil {
				return Outcome{}, err
			}
			to := st.Entry(stage).State

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
