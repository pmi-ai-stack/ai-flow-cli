package rollback

import (
	"context"
	"fmt"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// Repository is one in-scope repository's own name — enough for the
// git-revert text this package derives; no local path or instructions
// file is needed for a fixed sentence.
type Repository struct {
	Name string
}

// RepositoryRevert is one repository's own derived revert instruction
// — text only, never executed, no allowlist row (the same contract
// every addon's own Rollback already has).
type RepositoryRevert struct {
	Repo   string `json:"repo"`
	Revert string `json:"revert"`
}

// TargetResult is one target's own tool-level rollback: Addon.Rollback's
// Steps verbatim (already text-only, addon-ordered — never re-sorted
// here) and NonRevertible.
type TargetResult struct {
	Target        string   `json:"target"`
	Steps         []string `json:"steps"`
	NonRevertible []string `json:"non_revertible"`
}

// Result is rollback-results.json's own shape.
type Result struct {
	Repositories  []RepositoryRevert `json:"repositories"`
	Targets       []TargetResult     `json:"targets"`
	NonRevertible []string           `json:"non_revertible"`
	WatchList     []string           `json:"watch_list"`
}

// TargetInput is one target's own addon/env/changeset — everything
// Derive needs to call Addon.Rollback for real. Addon is nil when the
// target's own addon is not registered — Derive skips it, the same
// defensive-skip shape internal/cli/ops.go's renderOneTarget/
// driftOneTarget already use for an unregistered addon.
type TargetInput struct {
	Target    ops.Target
	Env       ops.Env
	Changeset ops.Changeset
	Addon     ops.Addon
}

// Derive builds Result: one RepositoryRevert per repo (regardless of
// whether any target belongs to it — a code change and an
// infrastructure change can both need reverting in the same
// repository), one TargetResult per target whose addon supports
// Capabilities().Rollback, and the flat NonRevertible/WatchList unions
// across every target.
func Derive(ctx context.Context, repos []Repository, targets []TargetInput) (Result, error) {
	res := Result{
		Repositories: make([]RepositoryRevert, 0, len(repos)),
		Targets:      make([]TargetResult, 0, len(targets)),
	}
	for _, r := range repos {
		res.Repositories = append(res.Repositories, RepositoryRevert{Repo: r.Name, Revert: revertText(r.Name)})
	}

	var nonRevertible, watchList []string

	for _, ti := range targets {
		if ti.Addon == nil || !ti.Addon.Capabilities().Rollback {
			continue
		}
		info, err := ti.Addon.Rollback(ctx, ti.Target, ti.Env, &ti.Changeset)
		if err != nil {
			return Result{}, fmt.Errorf("rollback: %s: %w", ops.TargetKey(ti.Target), err)
		}
		res.Targets = append(res.Targets, TargetResult{
			Target: ops.TargetKey(ti.Target), Steps: nonNilStrings(info.Steps), NonRevertible: nonNilStrings(info.NonRevertible),
		})
		nonRevertible = append(nonRevertible, info.NonRevertible...)
		for _, a := range ti.Changeset.Actions {
			if a.Action != ops.ActionNoop {
				watchList = append(watchList, a.Address)
			}
		}
	}

	sort.Slice(res.Targets, func(i, j int) bool { return res.Targets[i].Target < res.Targets[j].Target })
	res.NonRevertible = strutil.DedupSorted(nonRevertible)
	res.WatchList = strutil.DedupSorted(watchList)
	if res.NonRevertible == nil {
		res.NonRevertible = []string{}
	}
	if res.WatchList == nil {
		res.WatchList = []string{}
	}
	return res, nil
}

// nonNilStrings replaces a nil slice with an empty one — Addon.Rollback
// may return either for "no steps"/"nothing non-revertible", but the
// written artifact's own arrays must never be JSON null.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// revertText is the fixed, addon-independent git-revert instruction
// every in-scope repository gets — produced as text only, never
// executed, and needing no allowlist row (there is no merge commit to
// name until after ship, so this is necessarily generic guidance, not
// a literal runnable command).
func revertText(repo string) string {
	return fmt.Sprintf(
		"After this ticket's PR merges in %s, revert it with `git revert <merge-commit>` (or the PR's own revert control), then re-run the pipeline to confirm the revert is clean.",
		repo,
	)
}
