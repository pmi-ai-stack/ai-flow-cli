package ops

import (
	"cmp"
	"slices"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
)

// DetectInput is one repository ops detect walks: its checkout, and
// which of its own files changed. Changed is nil at scope "repo" (no
// diff exists yet, per Target.Changed's own doc comment) — a caller
// building this at the plan stage passes nil; at implement-completion
// it passes repo.json's in-scope files filtered to this repository.
type DetectInput struct {
	Repo    detect.Repo
	Changed []string
}

// DetectResult is ops-targets.json's own top-level shape. Field order
// matches the JSON-key alphabetical rule ("app_files" < "targets").
type DetectResult struct {
	AppFiles []string `json:"app_files"`
	Targets  []Target `json:"targets"`
}

// DetectAll runs every addon's Detect against every repo in repos,
// fills in the fields Addon.Detect does not set (Repo, Scope, Changed),
// sorts the result for byte stability, and computes AppFiles via
// internal/detect.AppFiles over every changed file no target claimed.
// scope is ScopeRepo or ScopeDiff — the same value for every target
// this call produces, since one ops detect call is always either "at
// plan" or "at implement completion," never a mix.
func DetectAll(addons []Addon, repos []DetectInput, scope string) (DetectResult, error) {
	var targets []Target
	var allChanged, claimed []string

	for _, ri := range repos {
		allChanged = append(allChanged, ri.Changed...)
		for _, a := range addons {
			found, err := a.Detect(ri.Repo, ri.Changed)
			if err != nil {
				return DetectResult{}, err
			}
			for _, t := range found {
				t.Repo = ri.Repo.Name
				t.Scope = scope
				if t.Files == nil {
					t.Files = []string{}
				}
				if scope == ScopeDiff {
					changed := len(t.Files) > 0
					t.Changed = &changed
				} else {
					t.Changed = nil
				}
				targets = append(targets, t)
				claimed = append(claimed, t.Files...)
			}
		}
	}

	slices.SortFunc(targets, func(a, b Target) int {
		return cmp.Or(cmp.Compare(a.Repo, b.Repo), cmp.Compare(a.Addon, b.Addon), cmp.Compare(a.Path, b.Path))
	})

	if targets == nil {
		targets = []Target{}
	}
	return DetectResult{Targets: targets, AppFiles: detect.AppFiles(allChanged, claimed)}, nil
}
