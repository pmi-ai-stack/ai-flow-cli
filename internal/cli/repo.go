package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/routing"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

func newRepoCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Route a ticket to its team and repositories",
	}
	cmd.AddCommand(newRepoLocateCmd(cio))
	cmd.AddCommand(newRepoConfirmCmd(cio))
	cmd.AddCommand(newRepoScopeCmd(cio))
	return cmd
}

// repoArtifactRepository mirrors one item of repo.schema.json's
// repositories[] exactly — additionalProperties is false there, so
// this struct's json tags are the schema's full property list.
type repoArtifactRepository struct {
	Name             string `json:"name"`
	LocalPath        string `json:"local_path,omitempty"`
	InstructionsFile string `json:"instructions_file"`
}

// repoArtifact mirrors repo.schema.json exactly.
type repoArtifact struct {
	Team         string                   `json:"team"`
	Repositories []repoArtifactRepository `json:"repositories"`
	InScopeFiles []string                 `json:"in_scope_files,omitempty"`
}

func toRepoArtifact(res routing.Result) repoArtifact {
	repos := make([]repoArtifactRepository, len(res.Repos))
	for i, r := range res.Repos {
		repos[i] = repoArtifactRepository{Name: r.Name, LocalPath: r.LocalPath, InstructionsFile: r.InstructionsFile}
	}
	return repoArtifact{Team: res.Team, Repositories: repos}
}

// namesOf extracts each item's name via name — shared by every command
// in this file and git.go that previews a repository list before
// writing anything (repoArtifactRepository and routing.RepoCandidate
// both carry a Name field, but have no common interface, hence the
// generic).
func namesOf[T any](items []T, name func(T) string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = name(item)
	}
	return out
}

// loadTicket reads ticketDir's ticket.json — missing_artifact if it
// has not been written yet (fetch never skips, so this only fires
// against a fixture that skipped it or a ticket directory driven by
// hand out of order).
func loadTicket(ticketDir string) (*ticketArtifact, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "ticket.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, &envelope.Error{Code: "missing_artifact", Message: "ticket.json not found; run ticket fetch first"}
	}
	var t ticketArtifact
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("cli: decode ticket.json: %w", err)
	}
	return &t, nil
}

// loadTextArtifact reads ticketDir/filename as plain text —
// missing_artifact if it has not been written yet. Shared by `git
// commit` (commit-message.md) and `pr create` (pr-body.md), both
// agent-authored files this phase only owns the mechanical read of.
func loadTextArtifact(ticketDir, filename string) (string, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, filename))
	if err != nil {
		return "", err
	}
	if data == nil {
		return "", &envelope.Error{Code: "missing_artifact", Message: filename + " not found"}
	}
	return string(data), nil
}

// loadRepoArtifact reads ticketDir's repo.json — missing_artifact if
// it has not been written yet (repo locate/confirm never skips).
func loadRepoArtifact(ticketDir string) (*repoArtifact, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "repo.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, &envelope.Error{Code: "missing_artifact", Message: "repo.json not found; run repo locate first"}
	}
	var r repoArtifact
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("cli: decode repo.json: %w", err)
	}
	return &r, nil
}

// driveCLIOnlyStage drives a CLI-only stage (fetch, locate, output)
// from pending to pass in one transaction, exactly as StageTable
// declares it. Generalizes ticket.go's driveFetchStage (phase 4) for
// this phase's second CLI-only stage, locate — see the plan's own
// note on why this is a fresh function rather than a ticket.go
// retrofit.
func driveCLIOnlyStage(f Flags, key string, stage handoff.Stage, command string) error {
	st, err := handoff.LoadStatus(f.Home, key)
	if err != nil {
		return err
	}
	from := st.Entry(stage).State

	switch from {
	case handoff.StatePending:
		if err := handoff.Start(st, stage); err != nil {
			return err
		}
	case handoff.StateRunning:
		// A prior crash mid-command; fall through to Complete below.
	default:
		return nil
	}

	if err := handoff.Complete(st, stage, false, handoff.GateInputs{}); err != nil {
		return err
	}
	to := st.Entry(stage).State

	return finalizeTransition(f.Home, key, stage, st, command, from, to, "")
}

// needsInputFor builds needs_input for the three exit-4 ambiguity
// codes this phase's commands can return (design decision 1):
// routing_ambiguous (its Details carry the candidates routing.Locate/
// Confirm already computed), dirty_worktree and base_diverged (both
// from GitOps.Prepare, asking the developer to resolve something
// outside ai-flow before retrying the exact same command). Any other
// error — including missing_repo_instructions, which has nothing to
// ask — returns nil.
func needsInputFor(err error, key string) *envelope.NeedsInput {
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		return nil
	}
	switch envErr.Code {
	case "routing_ambiguous":
		return &envelope.NeedsInput{
			Question:   fmt.Sprintf("Which repositories does %s touch?", key),
			Options:    envErr.Details,
			ResumeWith: fmt.Sprintf("repo confirm %s --repos <REPO>", key),
		}
	case "dirty_worktree":
		return &envelope.NeedsInput{
			Question:   "One or more repositories have uncommitted changes; commit, stash or discard them.",
			ResumeWith: "git prepare " + key,
		}
	case "base_diverged":
		return &envelope.NeedsInput{
			Question:   "One or more repositories' base branch has local commits not on origin; resolve locally.",
			ResumeWith: "git prepare " + key,
		}
	default:
		return nil
	}
}

// repoRoutingSimulateResult is `repo locate`/`repo confirm --simulate`'s
// result: routing already ran (it is read-only), but repo.json is not
// written and the locate stage is not driven.
type repoRoutingSimulateResult struct {
	Key          string   `json:"key"`
	Team         string   `json:"team"`
	Repositories []string `json:"repositories"`
}

func newRepoLocateCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "locate KEY",
		Short: "Route a ticket to its team and repositories, and complete the locate stage",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("repo locate", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			ticket, err := loadTicket(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			res, err := routing.Locate(cfg.Routing, key, ticket.Summary+"\n"+ticket.Description)
			if err != nil {
				return Outcome{NeedsInput: needsInputFor(err, key)}, err
			}

			if f.Simulate {
				return Outcome{Result: repoRoutingSimulateResult{Key: key, Team: res.Team, Repositories: namesOf(res.Repos, func(r routing.RepoCandidate) string { return r.Name })}}, nil
			}

			artifact := toRepoArtifact(res)
			if _, err := handoff.WriteArtifact(ticketDir, "repo.json", "repo", artifact); err != nil {
				return Outcome{}, err
			}
			if err := driveCLIOnlyStage(f, key, handoff.StageLocate, "repo locate"); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: artifact}, nil
		}),
	}
}

func newRepoConfirmCmd(cio cmdIO) *cobra.Command {
	var reposFlag []string
	cmd := &cobra.Command{
		Use:   "confirm KEY",
		Short: "Confirm a ticket's repositories explicitly, and complete the locate stage",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("repo confirm", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}

			res, err := routing.Confirm(cfg.Routing, reposFlag)
			if err != nil {
				return Outcome{}, err
			}

			if f.Simulate {
				return Outcome{Result: repoRoutingSimulateResult{Key: key, Team: res.Team, Repositories: namesOf(res.Repos, func(r routing.RepoCandidate) string { return r.Name })}}, nil
			}

			ticketDir := handoff.TicketDir(f.Home, key)
			artifact := toRepoArtifact(res)
			if _, err := handoff.WriteArtifact(ticketDir, "repo.json", "repo", artifact); err != nil {
				return Outcome{}, err
			}
			if err := driveCLIOnlyStage(f, key, handoff.StageLocate, "repo confirm"); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: artifact}, nil
		}),
	}
	cmd.Flags().StringArrayVar(&reposFlag, "repos", nil, "repositories the ticket touches (repeatable)")
	return cmd
}

func newRepoScopeCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "scope KEY",
		Short: "Record in-scope files from every repository's diff",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("repo scope", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			g := buildGitOps(cfg)
			var allFiles []string
			for _, r := range artifact.Repositories {
				diff, err := g.Diff(cmd.Context(), r.LocalPath, cfg.Host.DefaultBaseBranch)
				if err != nil {
					return Outcome{}, err
				}
				allFiles = append(allFiles, diff.Files...)
			}
			artifact.InScopeFiles = strutil.DedupSorted(allFiles)
			if artifact.InScopeFiles == nil {
				artifact.InScopeFiles = []string{}
			}

			if !f.Simulate {
				if _, err := handoff.WriteArtifact(ticketDir, "repo.json", "repo", artifact); err != nil {
					return Outcome{}, err
				}
			}
			return Outcome{Result: artifact}, nil
		}),
	}
}
