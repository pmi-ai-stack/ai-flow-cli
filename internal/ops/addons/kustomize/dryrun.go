package kustomize

import (
	"context"
	"fmt"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// DryRun resolves t's binding, then "kubectl diff --server-side -f ."
// in r.Dir (the branch rendering) as a presence-only fast path — exit
// 0 means every declared resource is a noop, exit 1 triggers the full
// get/apply --dry-run=server changeset build. Identical shape to
// kubectl's own DryRun (see doc.go for why no literal "-k" flag is
// used against the already-flattened rendering).
func (a *Addon) DryRun(ctx context.Context, t ops.Target, env ops.Env, r *ops.Rendered) (*ops.Changeset, error) {
	b, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}

	declared, err := parseManifests(r.Dir)
	if err != nil {
		return nil, err
	}

	diffArgs := append([]string{"diff", "--server-side", "-f", "."}, b.contextFlag()...)
	out, err := a.x.Run(ctx, "kubectl", diffArgs, exec.RunOpts{Dir: r.Dir, Timeout: a.timeout()})
	switch {
	case err == nil && out.ExitCode == diffNoDifference:
		return buildNoopChangeset(declared, t, env), nil
	case exec.IsToolFailed(err) && out.ExitCode == diffDifference:
		return a.buildChangedChangeset(ctx, r.Dir, b, declared, t, env)
	default:
		return nil, err
	}
}

func buildNoopChangeset(declared []resource, t ops.Target, env ops.Env) *ops.Changeset {
	actions := make([]ops.Action, 0, len(declared))
	for _, r := range declared {
		actions = append(actions, newAction(r, ops.ActionNoop, nil, t, env))
	}
	return &ops.Changeset{Actions: actions}
}

func (a *Addon) buildChangedChangeset(ctx context.Context, dir string, b binding, declared []resource, t ops.Target, env ops.Env) (*ops.Changeset, error) {
	beforeByAddress, err := a.getLiveByAddress(ctx, dir, b)
	if err != nil {
		return nil, err
	}

	applyArgs := append([]string{"apply", "-f", ".", "--dry-run=server", "-o", "json"}, b.contextFlag()...)
	applyOut, err := a.x.Run(ctx, "kubectl", applyArgs, exec.RunOpts{Dir: dir, Timeout: a.timeout()})
	if err != nil {
		return nil, err
	}
	after, err := decodeJSONStream([]byte(applyOut.Stdout))
	if err != nil {
		return nil, err
	}
	afterByAddress := indexByAddress(after)

	actions := make([]ops.Action, 0, len(declared))
	for _, r := range declared {
		afterObj, ok := afterByAddress[r.address()]
		if !ok {
			return nil, fmt.Errorf("kustomize: resource %s missing from dry-run apply output", r.address())
		}

		beforeObj, hadBefore := beforeByAddress[r.address()]
		if !hadBefore {
			actions = append(actions, newAction(r, ops.ActionCreate, nil, t, env))
			continue
		}

		fieldsChanged := topLevelDiff(stripVolatile(beforeObj), stripVolatile(afterObj))
		if len(fieldsChanged) == 0 {
			actions = append(actions, newAction(r, ops.ActionNoop, nil, t, env))
			continue
		}
		actions = append(actions, newAction(r, ops.ActionUpdate, fieldsChanged, t, env))
	}

	sort.Slice(actions, func(i, j int) bool { return actions[i].Address < actions[j].Address })
	return &ops.Changeset{Actions: actions}, nil
}

// newAction builds one ops.Action for r. Sensitive and Protected are
// always false — phase 23's internal/ops/risk owns both uniformly
// across every addon (kubectl's own phase-22 precedent).
func newAction(r resource, action ops.ActionKind, fieldsChanged []string, t ops.Target, env ops.Env) ops.Action {
	if fieldsChanged == nil {
		fieldsChanged = []string{}
	}
	return ops.Action{
		Action:        action,
		Address:       r.address(),
		Addon:         "kustomize",
		Env:           env.Name,
		FieldsChanged: fieldsChanged,
		Kind:          r.kind,
		KindClass:     classifyKind(r.kind),
		Name:          r.address(),
		Protected:     false,
		Sensitive:     false,
		Target:        ops.TargetKey(t),
		Tier:          env.Tier,
	}
}
