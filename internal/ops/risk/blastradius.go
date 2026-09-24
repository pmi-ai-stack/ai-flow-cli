package risk

import (
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// BlastRadiusEntry is one {env, namespace|workspace|job, kind} tuple
// the spec names, plus how many non-noop actions touched it. JSON tags
// live here for the same reason Contribution's own do (score.go) —
// fields declared in JSON-key alphabetical order: count, env, kind,
// scope.
type BlastRadiusEntry struct {
	Count int    `json:"count"`
	Env   string `json:"env"`
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
}

// BlastRadius is impact-results.json's own blast_radius shape: the
// touched-tuple set plus the spec's separate "counts per kind_class"
// aggregate. ops.KindClass's underlying type is string, so
// KindClassCounts marshals with each kind class as a plain JSON object
// key, exactly as if the map were declared map[string]int.
type BlastRadius struct {
	Entries         []BlastRadiusEntry    `json:"entries"`
	KindClassCounts map[ops.KindClass]int `json:"kind_class_counts"`
}

// scopeFor derives an action's own "namespace | workspace | job" value
// — design decision 4: no Addon method carries this generically, so it
// is derived per addon name from data already on hand. kubectl (and,
// from phase 26, helm/kustomize, expected to share the same address
// shape) encodes the namespace as Address's own leading path segment
// ("<namespace or "-">/<kind>/<name>" — the kubectl addon's own
// documented shape); terraform's workspace and jenkins's job URL live
// on the bound environment's own config, not on the action. Anything
// else (including the unbound environment) falls back to the
// environment name itself, or "-".
func scopeFor(a ops.Action, opsCfg *config.Ops) string {
	switch a.Addon {
	case "kubectl", "kustomize", "helm":
		if idx := strings.Index(a.Address, "/"); idx > 0 && a.Address[:idx] != "-" {
			return a.Address[:idx]
		}
		return "-"
	case "terraform":
		if opsCfg != nil {
			if env, ok := opsCfg.Environments[a.Env]; ok && env.Terraform != nil && env.Terraform.Workspace != "" {
				return env.Terraform.Workspace
			}
		}
	case "jenkins":
		if opsCfg != nil {
			if env, ok := opsCfg.Environments[a.Env]; ok && env.Jenkins != nil && env.Jenkins.URL != "" {
				return env.Jenkins.URL
			}
		}
	}
	if a.Env != "" {
		return a.Env
	}
	return "-"
}

// ComputeBlastRadius aggregates every non-noop action into the touched
// {env, scope, kind} set (a noop changed nothing, so it never counts
// as "touched") plus the separate kind_class count. Both are
// deterministic regardless of actions' own input order: entries are
// sorted by (env, scope, kind) before being returned.
func ComputeBlastRadius(actions []ops.Action, opsCfg *config.Ops) BlastRadius {
	counts := map[[3]string]int{}
	kindClassCounts := map[ops.KindClass]int{}

	for _, a := range actions {
		if a.Action == ops.ActionNoop {
			continue
		}
		key := [3]string{a.Env, scopeFor(a, opsCfg), a.Kind}
		counts[key]++
		kindClassCounts[a.KindClass]++
	}

	entries := make([]BlastRadiusEntry, 0, len(counts))
	for k, n := range counts {
		entries = append(entries, BlastRadiusEntry{Env: k[0], Scope: k[1], Kind: k[2], Count: n})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Env != entries[j].Env {
			return entries[i].Env < entries[j].Env
		}
		if entries[i].Scope != entries[j].Scope {
			return entries[i].Scope < entries[j].Scope
		}
		return entries[i].Kind < entries[j].Kind
	})

	return BlastRadius{Entries: entries, KindClassCounts: kindClassCounts}
}
