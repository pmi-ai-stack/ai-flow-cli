package helm

import (
	"context"
	"fmt"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

const (
	kubectlDiffNoDifference = 0
	kubectlDiffDifference   = 1
)

// DryRun resolves t's binding, then attempts the capability matrix's
// own primary path, "helm diff upgrade --detailed-exitcode" (a
// helm-diff plugin invocation whose exact exit-code/output contract is
// provisional — unverifiable against a real plugin binary in this
// environment, .ai/phase-26/plan.md §0.1/§1.2.1). A clean (err == nil)
// exit is trusted as "no differences" and produces an all-noop
// changeset with no further calls; ANY failure at all — a genuine
// diff-found exit, a missing-plugin error, or anything else — falls
// back unconditionally to the capability matrix's own explicit
// alternative, "template + kubectl diff": r (already helm template's
// own output, from Render) is compared against live state via
// kubectl's own verified get/apply --dry-run=server shape, reusing
// this package's own manifest.go/normalize.go. A wrong guess about
// helm-diff's contract therefore degrades to a mechanism this phase
// already verified for real, never to silently reporting nothing.
func (a *Addon) DryRun(ctx context.Context, t ops.Target, env ops.Env, r *ops.Rendered) (*ops.Changeset, error) {
	b, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}

	declared, err := parseManifests(r.Dir)
	if err != nil {
		return nil, err
	}

	meta, err := readChartMeta(t.Path)
	if err != nil {
		return nil, err
	}

	diffArgs := []string{"diff", "upgrade", meta.Name, t.Path, "--detailed-exitcode"}
	if vf, ok := findValuesFile(t.Path); ok {
		diffArgs = append(diffArgs, "-f", vf)
	}
	diffArgs = append(diffArgs, b.kubeContextFlag()...)
	if _, err := a.x.Run(ctx, "helm", diffArgs, exec.RunOpts{Timeout: a.timeout()}); err == nil {
		return buildNoopChangeset(declared, t, env), nil
	}

	return a.fallbackDryRun(ctx, r.Dir, b, declared, t, env)
}

// fallbackDryRun is the capability matrix's own "else template +
// kubectl diff" path — structurally identical to kubectl's own
// DryRun, against r.Dir (helm template's own output).
func (a *Addon) fallbackDryRun(ctx context.Context, dir string, b binding, declared []resource, t ops.Target, env ops.Env) (*ops.Changeset, error) {
	diffArgs := append([]string{"diff", "--server-side", "-f", "."}, b.contextFlag()...)
	out, err := a.x.Run(ctx, "kubectl", diffArgs, exec.RunOpts{Dir: dir, Timeout: a.timeout()})
	switch {
	case err == nil && out.ExitCode == kubectlDiffNoDifference:
		return buildNoopChangeset(declared, t, env), nil
	case exec.IsToolFailed(err) && out.ExitCode == kubectlDiffDifference:
		return a.buildChangedChangeset(ctx, dir, b, declared, t, env)
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
	getArgs := append([]string{"get", "-f", ".", "-o", "json", "--ignore-not-found"}, b.contextFlag()...)
	getOut, err := a.x.Run(ctx, "kubectl", getArgs, exec.RunOpts{Dir: dir, Timeout: a.timeout()})
	if err != nil {
		return nil, err
	}
	before, err := decodeJSONStream([]byte(getOut.Stdout))
	if err != nil {
		return nil, err
	}
	beforeByAddress := indexByAddress(before)

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
			return nil, fmt.Errorf("helm: resource %s missing from dry-run apply output", r.address())
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
// across every addon.
func newAction(r resource, action ops.ActionKind, fieldsChanged []string, t ops.Target, env ops.Env) ops.Action {
	if fieldsChanged == nil {
		fieldsChanged = []string{}
	}
	return ops.Action{
		Action:        action,
		Address:       r.address(),
		Addon:         "helm",
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
