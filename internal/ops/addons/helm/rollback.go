package helm

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// nonRevertibleClasses are the same four KindClass values the spec's
// own "protected" formula names for its delete/replace disjunct —
// reused here as this addon's own best-effort non-revertible signal,
// identical to kubectl's/terraform's own.
var nonRevertibleClasses = map[ops.KindClass]bool{
	ops.KindData:           true,
	ops.KindIAM:            true,
	ops.KindSecret:         true,
	ops.KindCRDOrNamespace: true,
}

// revisionLine finds every revision number in "helm history"'s own
// tabular stdout — a "REVISION\tUPDATED\t..." header followed by one
// numbered line per revision; the last match is the current (highest)
// revision.
var revisionLine = regexp.MustCompile(`(?m)^(\d+)\s`)

// Rollback calls "helm history <release>" **once per Rollback
// invocation, not once per action** — unlike kubectl's own per-action
// "kubectl rollout history" calls. A Helm release's history is
// release-scoped, not per-resource, so there is exactly one
// release-wide current revision to name (.ai/phase-26/plan.md §2
// design decision 5). The produced text instruction is the capability
// matrix's own literal words: "helm rollback <release> <revision>",
// never executed. delete/replace on a data/iam/secret/crd_or_namespace-
// class resource adds a NonRevertible entry, evaluated per action
// exactly like kubectl's own.
func (a *Addon) Rollback(ctx context.Context, t ops.Target, env ops.Env, cs *ops.Changeset) (*ops.RollbackInfo, error) {
	b, bindErr := resolveBinding(a.cfg, env.Name)

	actions := slices.Clone(cs.Actions)
	sort.Slice(actions, func(i, j int) bool { return actions[i].Address < actions[j].Address })

	info := &ops.RollbackInfo{
		Steps: []string{
			fmt.Sprintf("git revert the commits touching %s, then re-run `ops render --ref base` and `ops drift` against this target to confirm the reverted chart plans clean against live state.", t.Path),
		},
	}

	needsRollbackStep := false
	for _, act := range actions {
		if act.Action == ops.ActionUpdate || act.Action == ops.ActionReplace {
			needsRollbackStep = true
		}
		if (act.Action == ops.ActionDelete || act.Action == ops.ActionReplace) && nonRevertibleClasses[act.KindClass] {
			info.NonRevertible = append(info.NonRevertible, act.Address)
			info.Steps = append(info.Steps, fmt.Sprintf(
				"%s %s (%s) has no clean revert path via helm alone; recovery requires a %s-specific restore (a snapshot, a backup, or manual reconstruction) before any git revert can be considered complete.",
				act.Action, act.Address, act.KindClass, act.KindClass,
			))
		}
	}

	if needsRollbackStep {
		step, err := a.helmRollbackStep(ctx, t, b, bindErr)
		if err != nil {
			return nil, err
		}
		info.Steps = append(info.Steps, step)
	}
	return info, nil
}

// helmRollbackStep names the "helm rollback" text instruction for the
// release t.Path's own Chart.yaml names, with the revision to return
// to appended when it could be read.
func (a *Addon) helmRollbackStep(ctx context.Context, t ops.Target, b binding, bindErr error) (string, error) {
	meta, err := readChartMeta(t.Path)
	if err != nil {
		return "", err
	}

	rollback := fmt.Sprintf("helm rollback %s", meta.Name)
	if b.context != "" {
		rollback += " --kube-context " + b.context
	}

	if bindErr != nil {
		return rollback + " <revision> (no live context bound — confirm the current revision before running this).", nil
	}

	historyArgs := append([]string{"history", meta.Name}, b.kubeContextFlag()...)
	out, err := a.x.Run(ctx, "helm", historyArgs, exec.RunOpts{Timeout: a.timeout()})
	if err != nil {
		return fmt.Sprintf("%s <revision> (current revision could not be read: %v).", rollback, err), nil
	}

	if rev, ok := previousRevision(out.Stdout); ok {
		return fmt.Sprintf("%s %s — returns to the revision before the current one.", rollback, rev), nil
	}
	return rollback + " <revision>.", nil
}

// previousRevision returns the revision immediately before the
// highest one found in "helm history"'s own stdout (helm's own
// convention: rollback names the *target* revision explicitly, unlike
// kubectl's own "rollout undo" which implicitly returns to the
// previous one) — or false when fewer than two revisions are found.
func previousRevision(stdout string) (string, bool) {
	matches := revisionLine.FindAllStringSubmatch(stdout, -1)
	if len(matches) < 2 {
		return "", false
	}
	return matches[len(matches)-2][1], true
}
