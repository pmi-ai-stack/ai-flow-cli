package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/gitops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func newGitCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Every git operation the CLI performs on a ticket's repositories",
	}
	cmd.AddCommand(newGitPrepareCmd(cio))
	cmd.AddCommand(newGitCheckoutBaseCmd(cio))
	cmd.AddCommand(newGitCheckoutBranchCmd(cio))
	cmd.AddCommand(newGitDiffCmd(cio))
	cmd.AddCommand(newGitCommitCmd(cio))
	cmd.AddCommand(newGitPushCmd(cio))
	return cmd
}

// newExecer builds this layer's Execer. A package-level var, following
// internal/handoff/clock.go's injectable-clock pattern (Now), so a
// test can substitute a fake exec.Execer without ever touching a real
// subprocess — production always gets exec.NewRunner(), the binary's
// one real subprocess path.
var newExecer = func() exec.Execer { return exec.NewRunner() }

// buildGitOps wires newExecer() with the configured tool timeout — the
// one constructor every git subcommand and pr create calls.
func buildGitOps(cfg *config.Config) *gitops.GitOps {
	return gitops.New(newExecer(), cfg.Ops.ToolTimeout())
}

// requireShipApproved is the approval_required precondition design
// decisions 11/12 name: git commit, git push and pr create all refuse
// (exit 3, no needs_input — there is nothing to ask, the developer
// must run `stage approve KEY ship` first) unless a ship approval is
// already recorded. Checked before --simulate branches too, so a
// simulated preview never claims a command would succeed when the
// real one would be refused.
func requireShipApproved(f Flags, key string) error {
	st, err := handoff.LoadStatus(f.Home, key)
	if err != nil {
		return err
	}
	if !handoff.ShipApproved(st) {
		return &envelope.Error{
			Code:    "approval_required",
			Message: "git commit, git push and pr create require a recorded ship approval — run `stage approve KEY ship` first",
		}
	}
	return nil
}

// gitPrepareSimulateResult is `git prepare --simulate`'s result.
type gitPrepareSimulateResult struct {
	Key          string   `json:"key"`
	Base         string   `json:"base"`
	Branch       string   `json:"branch"`
	Worktree     bool     `json:"worktree"`
	Repositories []string `json:"repositories"`
}

func newGitPrepareCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "prepare KEY",
		Short: "Sync the base branch and create the ticket branch, per repository",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("git prepare", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
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
			ticket, err := loadTicket(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			if f.Simulate {
				return Outcome{Result: gitPrepareSimulateResult{
					Key: key, Base: cfg.Host.DefaultBaseBranch, Branch: ticket.BranchName,
					Worktree:     cfg.Defaults.Worktree,
					Repositories: namesOf(artifact.Repositories, func(r repoArtifactRepository) string { return r.Name }),
				}}, nil
			}

			g := buildGitOps(cfg)
			for i, r := range artifact.Repositories {
				res, err := g.Prepare(cmd.Context(), gitops.PrepareInput{
					LocalPath: r.LocalPath,
					Base:      cfg.Host.DefaultBaseBranch,
					Branch:    ticket.BranchName,
					Worktree:  cfg.Defaults.Worktree,
					Home:      f.Home,
					Key:       key,
					RepoName:  r.Name,
				})
				if err != nil {
					return Outcome{NeedsInput: needsInputFor(err, key)}, err
				}
				artifact.Repositories[i].LocalPath = res.WorkingPath
			}
			if _, err := handoff.WriteArtifact(ticketDir, "repo.json", "repo", artifact); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: artifact}, nil
		}),
	}
}

// gitCheckoutResult is checkout-base/checkout-branch's result.
type gitCheckoutResult struct {
	Key          string   `json:"key"`
	Ref          string   `json:"ref"`
	Repositories []string `json:"repositories"`
}

func newGitCheckoutBaseCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "checkout-base KEY",
		Short: "Switch every in-scope repository's HEAD to the base branch",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("git checkout-base", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			return runCheckout(cmd, args, true, (*gitops.GitOps).CheckoutBase)
		}),
	}
}

func newGitCheckoutBranchCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "checkout-branch KEY",
		Short: "Switch every in-scope repository's HEAD back to the ticket branch",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("git checkout-branch", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			return runCheckout(cmd, args, false, (*gitops.GitOps).CheckoutBranch)
		}),
	}
}

// runCheckout is checkout-base/checkout-branch's shared body: load
// repo.json and ticket.json, pick the target ref (the configured base
// branch, or the ticket's own branch name) once, then run do against
// every repository's current local_path with that one ref, in
// repo.json order.
func runCheckout(cmd *cobra.Command, args []string, useBase bool, do func(g *gitops.GitOps, ctx context.Context, dir, ref string) error) (Outcome, error) {
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
	ticket, err := loadTicket(ticketDir)
	if err != nil {
		return Outcome{}, err
	}
	ref := ticket.BranchName
	if useBase {
		ref = cfg.Host.DefaultBaseBranch
	}

	g := buildGitOps(cfg)
	names := make([]string, len(artifact.Repositories))
	for i, r := range artifact.Repositories {
		if !f.Simulate {
			if err := do(g, cmd.Context(), r.LocalPath, ref); err != nil {
				return Outcome{}, err
			}
		}
		names[i] = r.Name
	}
	return Outcome{Result: gitCheckoutResult{Key: key, Ref: ref, Repositories: names}}, nil
}

// gitDiffRepoResult is one repository's entry in `git diff`'s result.
type gitDiffRepoResult struct {
	Repo  string   `json:"repo"`
	Files []string `json:"files"`
	Diff  string   `json:"diff"`
}

type gitDiffResult struct {
	Key          string              `json:"key"`
	Repositories []gitDiffRepoResult `json:"repositories"`
}

func newGitDiffCmd(cio cmdIO) *cobra.Command {
	var repoFlag string
	cmd := &cobra.Command{
		Use:   "diff KEY",
		Short: "Diff every in-scope repository (or one, with --repo) against its base branch",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("git diff", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
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

			targets := artifact.Repositories
			if repoFlag != "" {
				targets = nil
				for _, r := range artifact.Repositories {
					if r.Name == repoFlag {
						targets = append(targets, r)
					}
				}
				if len(targets) == 0 {
					return Outcome{}, &envelope.Error{Code: "usage", Message: "--repo " + repoFlag + " is not one of repo.json's repositories"}
				}
			}

			g := buildGitOps(cfg)
			result := gitDiffResult{Key: key}
			for _, r := range targets {
				d, err := g.Diff(cmd.Context(), r.LocalPath, cfg.Host.DefaultBaseBranch)
				if err != nil {
					return Outcome{}, err
				}
				result.Repositories = append(result.Repositories, gitDiffRepoResult{Repo: r.Name, Files: d.Files, Diff: d.Diff})
			}
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "narrow to one repository by name")
	return cmd
}

// gitCommitRepoResult is one repository's entry in `git commit`'s result.
type gitCommitRepoResult struct {
	Repo      string `json:"repo"`
	Committed bool   `json:"committed"`
}

// gitCommitResult is `git commit`'s result — also `--simulate`'s: the
// same shape, with Committed reporting whether a repo is dirty
// (would-commit) rather than whether it was actually committed.
type gitCommitResult struct {
	Key          string                `json:"key"`
	Repositories []gitCommitRepoResult `json:"repositories"`
}

func newGitCommitCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "commit KEY",
		Short: "Commit every in-scope repository's changes with commit-message.md",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("git commit", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			if err := requireShipApproved(f, key); err != nil {
				return Outcome{}, err
			}

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			message, err := loadTextArtifact(ticketDir, "commit-message.md")
			if err != nil {
				return Outcome{}, err
			}

			g := buildGitOps(cfg)
			if f.Simulate {
				result := gitCommitResult{Key: key}
				for _, r := range artifact.Repositories {
					dirty, err := g.IsDirty(cmd.Context(), r.LocalPath)
					if err != nil {
						return Outcome{}, err
					}
					result.Repositories = append(result.Repositories, gitCommitRepoResult{Repo: r.Name, Committed: dirty})
				}
				return Outcome{Result: result}, nil
			}

			result := gitCommitResult{Key: key}
			for _, r := range artifact.Repositories {
				committed, err := g.Commit(cmd.Context(), r.LocalPath, message)
				if err != nil {
					return Outcome{}, err
				}
				result.Repositories = append(result.Repositories, gitCommitRepoResult{Repo: r.Name, Committed: committed})
			}
			return Outcome{Result: result}, nil
		}),
	}
}

// gitPushResult is `git push`'s result.
type gitPushResult struct {
	Key          string   `json:"key"`
	Repositories []string `json:"repositories"`
}

func newGitPushCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "push KEY",
		Short: "Push every in-scope repository's ticket branch",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("git push", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			if err := requireShipApproved(f, key); err != nil {
				return Outcome{}, err
			}

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			ticket, err := loadTicket(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			g := buildGitOps(cfg)
			names := make([]string, len(artifact.Repositories))
			for i, r := range artifact.Repositories {
				if !f.Simulate {
					if err := g.Push(cmd.Context(), r.LocalPath, ticket.BranchName); err != nil {
						return Outcome{}, err
					}
				}
				names[i] = r.Name
			}
			return Outcome{Result: gitPushResult{Key: key, Repositories: names}}, nil
		}),
	}
}
