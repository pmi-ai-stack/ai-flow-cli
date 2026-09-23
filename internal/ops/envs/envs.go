// Package envs binds a target to a live environment from
// config.ops.environments, in the documented precedence order:
// config.ops.target_map, then path convention, then
// config.ops.default_environment (§ OPS: targets, changeset, stages,
// addons — "Environment"). It imports neither internal/ops nor any
// Addon — Bind takes plain strings so internal/ops stays free of a
// dependency on internal/config, and internal/cli's own ops detect
// command fills a Target's Environment/Tier from this package's
// Result after internal/ops.DetectAll returns.
package envs

import (
	"fmt"
	"path"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// Result is Bind's resolved environment. Name is "" for the unbound
// environment — no addon binding exists at all, so Tier is always
// "nonprod" in that case regardless of what cfg names.
type Result struct {
	Name string
	Tier string
}

// Bind resolves one target (identified by its repo and its path
// relative to that repo's checkout, plus any of its own changed
// files) to a live environment.
//
// hasLiveCounterpart is false for an addon whose targets have no live
// counterpart at all (a GitHub Actions workflow file, an unconfigured
// Jenkins job) — Bind then returns the unbound environment without
// consulting cfg at all, matching the spec's own rule exactly. No
// addon in this phase's registry sets this to false (none exists);
// phase 27 is the first caller that will.
//
// The "workspace name" convention the spec also names for Terraform
// specifically is not implemented — it needs live `terraform workspace
// show` state no addon supplies before phase 21 (the phase-18 plan's
// design decision 3). The two path-based conventions
// (envs/<name>/, overlays/<name>/, values-<name>.yaml) are checked
// against path and every entry of files, since a Helm/Kustomize
// target's own values file is often one of its changed files rather
// than its own Path.
func Bind(cfg *config.Config, repo, path string, files []string, hasLiveCounterpart bool) (Result, error) {
	if !hasLiveCounterpart {
		return Result{Tier: "nonprod"}, nil
	}

	if envName, ok := cfg.Ops.TargetMap[repo+":"+path]; ok {
		return resultFor(cfg, envName), nil
	}

	switch matches := conventionMatches(cfg, path, files); len(matches) {
	case 0:
		return resultFor(cfg, cfg.Ops.DefaultEnvironment), nil
	case 1:
		return resultFor(cfg, matches[0]), nil
	default:
		return Result{}, ambiguousError(matches)
	}
}

func resultFor(cfg *config.Config, name string) Result {
	return Result{Name: name, Tier: cfg.Ops.Environments[name].Tier}
}

func ambiguousError(candidates []string) *envelope.Error {
	return &envelope.Error{
		Code:    "env_ambiguous",
		Message: "the target's path matches more than one configured environment's naming convention",
		Details: candidates,
	}
}

// conventionMatches returns every distinct, sorted environment name
// whose documented convention matches path or one of files.
func conventionMatches(cfg *config.Config, path string, files []string) []string {
	candidates := append([]string{path}, files...)

	var names []string
	for name := range cfg.Ops.Environments {
		for _, c := range candidates {
			if matchesConvention(c, name) {
				names = append(names, name)
				break
			}
		}
	}
	return strutil.DedupSorted(names)
}

func matchesConvention(candidate, envName string) bool {
	return strings.Contains(candidate, "envs/"+envName+"/") ||
		strings.Contains(candidate, "overlays/"+envName+"/") ||
		path.Base(candidate) == fmt.Sprintf("values-%s.yaml", envName)
}
