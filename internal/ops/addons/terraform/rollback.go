package terraform

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// nonRevertibleClasses are the same four KindClass values the spec's
// own "protected" formula names for its delete/replace disjunct —
// reused here as this addon's own best-effort non-revertible signal
// (phase 23's internal/ops/rollback owns the final, cross-addon
// non_revertible[] derivation and may refine or override this).
var nonRevertibleClasses = map[ops.KindClass]bool{
	ops.KindData:           true,
	ops.KindIAM:            true,
	ops.KindSecret:         true,
	ops.KindCRDOrNamespace: true,
}

// Rollback derives the revert path as text only, running no subprocess
// at all (the spec's own words for the whole capability: "produced as
// text and never executed; they have no allowlist row" — the
// phase-21 plan's design decision 7). A fresh "reverse plan" subprocess
// would need to already know the state that exists *after* the
// branch's own change applies, which nothing before ship can know;
// the textual instruction to re-run `ops render --ref base` + `ops
// drift` after any real revert is the closest honest analogue.
func (a *Addon) Rollback(_ context.Context, t ops.Target, _ ops.Env, cs *ops.Changeset) (*ops.RollbackInfo, error) {
	actions := slices.Clone(cs.Actions)
	sort.Slice(actions, func(i, j int) bool { return actions[i].Address < actions[j].Address })

	info := &ops.RollbackInfo{
		Steps: []string{
			fmt.Sprintf("git revert the commits touching %s, then re-run `ops render --ref base` and `ops drift` against this target to confirm the reverted configuration plans clean against live state.", t.Path),
		},
	}
	for _, a := range actions {
		if (a.Action != ops.ActionDelete && a.Action != ops.ActionReplace) || !nonRevertibleClasses[a.KindClass] {
			continue
		}
		info.NonRevertible = append(info.NonRevertible, a.Address)
		info.Steps = append(info.Steps, fmt.Sprintf(
			"%s %s (%s) has no clean revert path via terraform alone; recovery requires a %s-specific restore (a snapshot, a backup, or manual reconstruction) before any git revert can be considered complete.",
			a.Action, a.Address, a.KindClass, a.KindClass,
		))
	}
	return info, nil
}
