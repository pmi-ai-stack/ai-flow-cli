package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/gitops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/envs"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

func newOpsCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ops",
		Short: "OPS: detect infrastructure targets, list addons and the execution allowlist",
	}
	cmd.AddCommand(newOpsDetectCmd(cio))
	cmd.AddCommand(newOpsRenderCmd(cio))
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

// loadOpsTargets reads ticketDir's ops-targets.json — missing_artifact
// if it has not been written yet, the same tolerant-of-absence-as-error
// shape loadRepoArtifact/loadTicket already use for a prerequisite
// artifact.
func loadOpsTargets(ticketDir string) (*ops.DetectResult, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "ops-targets.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, &envelope.Error{Code: "missing_artifact", Message: "ops-targets.json not found; run ops detect first"}
	}
	var r ops.DetectResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("cli: decode ops-targets.json: %w", err)
	}
	return &r, nil
}

type opsRenderResultTarget struct {
	Target string `json:"target"`
	Hash   string `json:"hash"`
	Dir    string `json:"dir"`
}

type opsRenderResult struct {
	Key     string                  `json:"key"`
	Ref     string                  `json:"ref"`
	Targets []opsRenderResultTarget `json:"targets"`
}

func newOpsRenderCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render KEY --ref branch|base",
		Short: "Materialize OPS targets from a git ref, memoized by tree hash",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops render", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			refFlag, _ := cmd.Flags().GetString("ref")
			targetFlag, _ := cmd.Flags().GetString("target")
			ref, err := parseRef(refFlag)
			if err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			ticketDir := handoff.TicketDir(f.Home, key)
			repoArt, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			targetsArt, err := loadOpsTargets(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			targets := targetsArt.Targets
			if targetFlag != "" {
				var filtered []ops.Target
				for _, t := range targets {
					if ops.TargetKey(t) == targetFlag {
						filtered = append(filtered, t)
					}
				}
				if len(filtered) == 0 {
					return Outcome{}, &envelope.Error{Code: "missing_artifact", Message: fmt.Sprintf("no target %q in ops-targets.json", targetFlag)}
				}
				targets = filtered
			}

			reg := ops.DefaultRegistry()
			result := opsRenderResult{Key: key, Ref: refFlag}
			for _, t := range targets {
				rt, err := renderOneTarget(cmd.Context(), f, key, repoArt, t, ref, reg)
				if err != nil {
					return Outcome{}, err
				}
				if rt != nil {
					result.Targets = append(result.Targets, *rt)
				}
			}
			if result.Targets == nil {
				result.Targets = []opsRenderResultTarget{}
			}
			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().String("ref", "", "which tree to materialize: branch or base")
	cmd.Flags().String("target", "", "render only the target named <repo>:<path>")
	return cmd
}

func parseRef(s string) (ops.Ref, error) {
	switch s {
	case string(ops.RefBranch):
		return ops.RefBranch, nil
	case string(ops.RefBase):
		return ops.RefBase, nil
	default:
		return "", &envelope.Error{Code: "usage", Message: fmt.Sprintf("--ref must be %q or %q, got %q", ops.RefBranch, ops.RefBase, s)}
	}
}

// renderOneTarget resolves t's source directory for ref (branch:
// repoArt's own repository LocalPath; base: gitops.BaseWorktreePath,
// which `git prepare` always creates) and, unless simulating, runs
// internal/ops/render.Render through t's own addon, looked up in reg
// (the caller's production call site always passes
// ops.DefaultRegistry() — this parameter exists so this function's
// own orchestration is directly unit-testable with a fixture addon,
// without adding a registry-injection hook to the shipped `ops render`
// command itself; see the phase-19 plan's design decision 8/phase 18's
// own precedent on why no such hook exists there). A nil result with a
// nil error means t's addon is not registered in reg — unreachable in
// the shipped binary today (DefaultRegistry() is empty before phase
// 21, so ops-targets.json never names an unregistered addon), kept as
// a defensive skip rather than an error since ops render KEY with no
// --target must stay a no-op, not fail, while no addon exists at all.
func renderOneTarget(ctx context.Context, f Flags, key string, repoArt *repoArtifact, t ops.Target, ref ops.Ref, reg *ops.Registry) (*opsRenderResultTarget, error) {
	addon, ok := reg.Lookup(t.Addon)
	if !ok {
		return nil, nil
	}

	var repo *repoArtifactRepository
	for i, r := range repoArt.Repositories {
		if r.Name == t.Repo {
			repo = &repoArt.Repositories[i]
			break
		}
	}
	if repo == nil {
		return nil, &envelope.Error{Code: "missing_artifact", Message: fmt.Sprintf("repo.json names no repository %q for target %q", t.Repo, ops.TargetKey(t))}
	}

	var root string
	switch ref {
	case ops.RefBranch:
		root = repo.LocalPath
	case ops.RefBase:
		root = gitops.BaseWorktreePath(f.Home, key, repo.Name)
	}
	if !dirExists(root) {
		return nil, &envelope.Error{Code: "missing_artifact", Message: fmt.Sprintf("no %s checkout at %s for %s; run git prepare first", ref, root, t.Repo)}
	}
	sourceDir := filepath.Join(root, t.Path)

	if f.Simulate {
		return &opsRenderResultTarget{Target: ops.TargetKey(t)}, nil
	}

	res, err := render.Render(ctx, render.Options{
		Addon: addon, Target: t, Ref: ref, SourceDir: sourceDir, Home: f.Home, Key: key,
	})
	if err != nil {
		return nil, err
	}
	return &opsRenderResultTarget{Target: ops.TargetKey(t), Hash: res.Hash, Dir: res.RedactedDir}, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
