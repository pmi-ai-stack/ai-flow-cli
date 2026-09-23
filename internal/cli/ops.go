package cli

import (
	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/envs"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func newOpsCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ops",
		Short: "OPS: detect infrastructure targets, list addons and the execution allowlist",
	}
	cmd.AddCommand(newOpsDetectCmd(cio))
	cmd.AddCommand(newOpsAddonsCmd(cio))
	cmd.AddCommand(newOpsAllowlistCmd(cio))
	return cmd
}

// opsDetectResult is `ops detect`'s own envelope result — the same
// shape as ops-targets.json plus the ticket key, so the command's
// output is useful without opening the file it just wrote.
type opsDetectResult struct {
	Key      string       `json:"key"`
	AppFiles []string     `json:"app_files"`
	Targets  []ops.Target `json:"targets"`
}

func newOpsDetectCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "detect KEY",
		Short: "Detect OPS targets across every in-scope repository and write ops-targets.json",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops detect", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
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

			result, err := runOpsDetect(cfg, artifact, scopeFor(artifact))
			if err != nil {
				return Outcome{NeedsInput: needsInputFor(err, key)}, err
			}

			if !f.Simulate {
				if _, err := handoff.WriteArtifact(ticketDir, "ops-targets.json", "ops-targets", result); err != nil {
					return Outcome{}, err
				}
			}
			return Outcome{Result: opsDetectResult{Key: key, AppFiles: result.AppFiles, Targets: result.Targets}}, nil
		}),
	}
}

// scopeFor answers ops detect's own scope without a --scope flag the
// developer could get wrong: repo.json's InScopeFiles is nil until
// `repo scope` has run at least once (the same "no diff exists yet"
// condition the spec's own Target.Changed doc names), so its presence
// is exactly the plan-vs-implement-or-later signal.
func scopeFor(artifact *repoArtifact) string {
	if artifact.InScopeFiles != nil {
		return ops.ScopeDiff
	}
	return ops.ScopeRepo
}

// runOpsDetect runs internal/ops.DetectAll with the production
// (currently empty — no addon exists before phase 21) registry over
// every repository repoArtifact names, then binds each target's
// environment via internal/ops/envs.
//
// At scope diff, every repository's Detect call receives the same
// flat artifact.InScopeFiles — repo.json's own diff result, already
// computed by `repo scope` and carrying no per-repository
// qualification of its own (a property this phase inherits, not one
// it introduces: phase 5's own "Decides" line already named the
// monorepo question open). Re-deriving a fresh, repo-scoped diff here
// via a second `git diff` call was considered and rejected: it would
// make this stage-machine hook depend on a real git checkout existing
// at every repository's LocalPath, which no generic stage-transition
// test sets up (confirmed by running the existing suite against a
// first draft that did exactly that — three tests failed with a real
// "not a git repository" subprocess error). An addon's own Detect
// still only claims files that fall under its own target's path, so
// the one visible cost is a false-positive risk when two different
// repositories happen to share a target path exactly — the same
// class of monorepo ambiguity already accepted elsewhere.
func runOpsDetect(cfg *config.Config, artifact *repoArtifact, scope string) (ops.DetectResult, error) {
	var changed []string
	if scope == ops.ScopeDiff {
		changed = artifact.InScopeFiles
	}

	inputs := make([]ops.DetectInput, 0, len(artifact.Repositories))
	for _, r := range artifact.Repositories {
		inputs = append(inputs, ops.DetectInput{Repo: toDetectRepo(r), Changed: changed})
	}

	result, err := ops.DetectAll(ops.DefaultRegistry().Addons(), inputs, scope)
	if err != nil {
		return ops.DetectResult{}, err
	}

	for i, t := range result.Targets {
		bound, err := envs.Bind(cfg, t.Repo, t.Path, t.Files, true)
		if err != nil {
			return ops.DetectResult{}, err
		}
		if bound.Name != "" {
			result.Targets[i].Environment = &bound.Name
		}
		result.Targets[i].Tier = &bound.Tier
	}
	return result, nil
}

type opsAddonsResult struct {
	Addons []opsAddonEntry `json:"addons"`
}

type opsAddonEntry struct {
	Name         string           `json:"name"`
	Capabilities ops.Capabilities `json:"capabilities"`
}

func newOpsAddonsCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "addons",
		Short: "List every addon this binary knows about, and their capabilities",
		Args:  cobra.NoArgs,
		RunE: wrap("ops addons", cio.stdout, cio.exitCode, func(cmd *cobra.Command, _ []string) (Outcome, error) {
			var entries []opsAddonEntry
			for _, a := range ops.DefaultRegistry().Addons() {
				entries = append(entries, opsAddonEntry{Name: a.Name(), Capabilities: a.Capabilities()})
			}
			if entries == nil {
				entries = []opsAddonEntry{}
			}
			return Outcome{Result: opsAddonsResult{Addons: entries}}, nil
		}),
	}
}

type opsAllowlistRow struct {
	Tool           string     `json:"tool"`
	Verb           []string   `json:"verb"`
	RequiredFlags  []string   `json:"required_flags,omitempty"`
	ForbiddenFlags []string   `json:"forbidden_flags,omitempty"`
	Mutating       bool       `json:"mutating"`
	Class          exec.Class `json:"class"`
}

type opsAllowlistResult struct {
	Rows []opsAllowlistRow `json:"rows"`
}

func newOpsAllowlistCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "allowlist",
		Short: "Print the execution allowlist: every external tool ai-flow may run",
		Args:  cobra.NoArgs,
		RunE: wrap("ops allowlist", cio.stdout, cio.exitCode, func(cmd *cobra.Command, _ []string) (Outcome, error) {
			rows := make([]opsAllowlistRow, 0, len(exec.Table))
			for _, r := range exec.Table {
				rows = append(rows, opsAllowlistRow{
					Tool: r.Tool, Verb: r.Verb, RequiredFlags: r.RequiredFlags,
					ForbiddenFlags: r.ForbiddenFlags, Mutating: r.Mutating, Class: r.Class,
				})
			}
			return Outcome{Result: opsAllowlistResult{Rows: rows}}, nil
		}),
	}
}
