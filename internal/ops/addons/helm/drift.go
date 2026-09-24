package helm

import (
	"context"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Drift compares baseRender (helm template's own output, from the base
// ref) against t's live environment via "helm get manifest" — the
// capability matrix's own literal words, "against the base template."
// Unlike kubectl's/kustomize's own Drift, there is no presence-only
// fast path: "helm get manifest" carries no diff-style exit-code
// signal of its own, so every call goes straight to the structured
// comparison (.ai/phase-26/plan.md §2 design decision 3).
func (a *Addon) Drift(ctx context.Context, t ops.Target, env ops.Env, baseRender *ops.Rendered) (*ops.DriftReport, error) {
	b, err := resolveBinding(a.cfg, env.Name)
	if err != nil {
		return nil, err
	}

	meta, err := readChartMeta(t.Path)
	if err != nil {
		return nil, err
	}

	declared, err := parseManifests(baseRender.Dir)
	if err != nil {
		return nil, err
	}

	args := append([]string{"get", "manifest", meta.Name}, b.kubeContextFlag()...)
	out, err := a.x.Run(ctx, "helm", args, exec.RunOpts{Timeout: a.timeout()})
	if err != nil {
		return nil, err
	}
	live, err := parseManifestBytes([]byte(out.Stdout), "helm get manifest")
	if err != nil {
		return nil, err
	}
	liveByAddress := indexResourcesByAddress(live)

	var resources []string
	for _, r := range declared {
		liveObj, ok := liveByAddress[r.address()]
		if !ok || len(topLevelDiff(stripVolatile(r.doc), stripVolatile(liveObj.doc))) > 0 {
			resources = append(resources, r.address())
		}
	}
	sort.Strings(resources)
	if resources == nil {
		resources = []string{}
	}

	drift := ops.DriftNone
	if len(resources) > 0 {
		drift = ops.DriftPresent
	}
	return &ops.DriftReport{Drift: drift, Resources: resources}, nil
}

func indexResourcesByAddress(resources []resource) map[string]resource {
	out := make(map[string]resource, len(resources))
	for _, r := range resources {
		out[r.address()] = r
	}
	return out
}
