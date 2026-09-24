package kubectl

import (
	"context"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// diffNoDifference/diffDifference are "kubectl diff"'s own documented
// exit codes — 0 when the live state matches, 1 when it differs.
// Anything else is a real tool_failed, propagated unchanged (the same
// exit-code-only reading terraform's own Drift already established for
// "plan -detailed-exitcode" — .ai/phase-22/plan.md §0.9: diff's own
// stdout text is external-diff-tool-dependent and never parsed here).
const (
	diffNoDifference = 0
	diffDifference   = 1
)

// Drift compares baseRender against t's live environment:
// resolveBinding first (env.Name == "" or an environment with no kube
// binding is ops.ErrLiveCredentialsMissing before any subprocess
// runs — internal/cli/ops_drift.go's own existing code degrades that
// to drift: "offline"), then "kubectl diff -f . --context <context>"
// in baseRender.Dir as a presence-only fast path (exit 0 → DriftNone,
// no further calls); exit 1 → "kubectl get -f . -o json
// --ignore-not-found --context <context>" in the same directory,
// correlated by address against baseRender's own declared manifests —
// a resource with no live counterpart, or whose declared fields differ
// from the live object's (after stripping server-set noise), is
// drifted.
func (a *Addon) Drift(ctx context.Context, _ ops.Target, env ops.Env, baseRender *ops.Rendered) (*ops.DriftReport, error) {
	b, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}

	diffArgs := append([]string{"diff", "-f", "."}, b.contextFlag()...)
	opts := exec.RunOpts{Dir: baseRender.Dir, Timeout: a.timeout()}
	out, err := a.x.Run(ctx, "kubectl", diffArgs, opts)
	switch {
	case err == nil && out.ExitCode == diffNoDifference:
		return &ops.DriftReport{Drift: ops.DriftNone, Resources: []string{}}, nil
	case exec.IsToolFailed(err) && out.ExitCode == diffDifference:
		resources, dErr := a.driftResources(ctx, baseRender.Dir, b)
		if dErr != nil {
			return nil, dErr
		}
		return &ops.DriftReport{Drift: ops.DriftPresent, Resources: resources}, nil
	default:
		return nil, err
	}
}

func (a *Addon) driftResources(ctx context.Context, dir string, b binding) ([]string, error) {
	declared, err := parseManifests(dir)
	if err != nil {
		return nil, err
	}

	liveByAddress, err := a.getLiveByAddress(ctx, dir, b)
	if err != nil {
		return nil, err
	}

	var resources []string
	for _, r := range declared {
		liveObj, ok := liveByAddress[r.address()]
		if !ok || len(topLevelDiff(stripVolatile(r.doc), stripVolatile(liveObj))) > 0 {
			resources = append(resources, r.address())
		}
	}
	sort.Strings(resources)
	if resources == nil {
		resources = []string{}
	}
	return resources, nil
}

// indexByAddress keys every decoded live object by its own
// "<namespace or "-">/<kind>/<name>" address — the same identity
// resource.address() computes for a declared manifest, so the two can
// be correlated directly.
func indexByAddress(objs []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(objs))
	for _, o := range objs {
		out[objectAddress(o)] = o
	}
	return out
}

func objectAddress(o map[string]any) string {
	kind, _ := o["kind"].(string)
	var namespace, name string
	if meta, ok := o["metadata"].(map[string]any); ok {
		namespace, _ = meta["namespace"].(string)
		name, _ = meta["name"].(string)
	}
	return buildAddress(namespace, kind, name)
}
