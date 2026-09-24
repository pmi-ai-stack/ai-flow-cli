package kustomize

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// nonRevertibleClasses are the same four KindClass values the spec's
// own "protected" formula names for its delete/replace disjunct —
// identical to kubectl's own (rollback "as kubectl," per the spec).
var nonRevertibleClasses = map[ops.KindClass]bool{
	ops.KindData:           true,
	ops.KindIAM:            true,
	ops.KindSecret:         true,
	ops.KindCRDOrNamespace: true,
}

var revisionLine = regexp.MustCompile(`(?m)^(\d+)\s`)

// Rollback derives, for every rollout-capable resource with an
// update/replace action, a real, read-only "kubectl rollout history"
// call to name the revision the produced "kubectl rollout undo" text
// instruction would return to — identical to kubectl's own Rollback
// (the spec's own words: "as kubectl").
func (a *Addon) Rollback(ctx context.Context, t ops.Target, env ops.Env, cs *ops.Changeset) (*ops.RollbackInfo, error) {
	b, bindErr := resolveBinding(a.cfg, env.Name)

	actions := slices.Clone(cs.Actions)
	sort.Slice(actions, func(i, j int) bool { return actions[i].Address < actions[j].Address })

	info := &ops.RollbackInfo{
		Steps: []string{
			fmt.Sprintf("git revert the commits touching %s, then re-run `ops render --ref base` and `ops drift` against this target to confirm the reverted manifests plan clean against live state.", t.Path),
		},
	}

	for _, act := range actions {
		if (act.Action == ops.ActionUpdate || act.Action == ops.ActionReplace) && rolloutCapable(act.Kind) {
			info.Steps = append(info.Steps, a.rolloutStep(ctx, act, b, bindErr))
		}
		if (act.Action == ops.ActionDelete || act.Action == ops.ActionReplace) && nonRevertibleClasses[act.KindClass] {
			info.NonRevertible = append(info.NonRevertible, act.Address)
			info.Steps = append(info.Steps, fmt.Sprintf(
				"%s %s (%s) has no clean revert path via kubectl alone; recovery requires a %s-specific restore (a snapshot, a backup, or manual reconstruction) before any git revert can be considered complete.",
				act.Action, act.Address, act.KindClass, act.KindClass,
			))
		}
	}
	return info, nil
}

func (a *Addon) rolloutStep(ctx context.Context, act ops.Action, b binding, bindErr error) string {
	namespace, kind, name := splitAddress(act.Address)
	undo := fmt.Sprintf("kubectl rollout undo %s/%s", kind, name)
	if namespace != "" {
		undo += " -n " + namespace
	}
	if b.context != "" {
		undo += " --context " + b.context
	}

	if bindErr != nil {
		return undo + " (no live context bound — confirm the current revision before running this)."
	}

	historyArgs := []string{"rollout", "history", kind + "/" + name}
	if namespace != "" {
		historyArgs = append(historyArgs, "-n", namespace)
	}
	historyArgs = append(historyArgs, b.contextFlag()...)
	out, err := a.x.Run(ctx, "kubectl", historyArgs, exec.RunOpts{Timeout: a.timeout()})
	if err != nil {
		return fmt.Sprintf("%s (current revision could not be read: %v).", undo, err)
	}

	if rev, ok := currentRevision(out.Stdout); ok {
		return fmt.Sprintf("%s — current revision is %s; this instruction returns to the revision before it.", undo, rev)
	}
	return undo + "."
}

func splitAddress(address string) (namespace, kind, name string) {
	parts := strings.SplitN(address, "/", 3)
	if len(parts) != 3 {
		return "", "", ""
	}
	namespace = parts[0]
	if namespace == "-" {
		namespace = ""
	}
	return namespace, parts[1], parts[2]
}

func currentRevision(stdout string) (string, bool) {
	matches := revisionLine.FindAllStringSubmatch(stdout, -1)
	if len(matches) == 0 {
		return "", false
	}
	return matches[len(matches)-1][1], true
}
