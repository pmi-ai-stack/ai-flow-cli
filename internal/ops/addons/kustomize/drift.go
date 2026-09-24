package kustomize

import (
	"context"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

const (
	diffNoDifference = 0
	diffDifference   = 1
)

// Drift compares baseRender (kustomize build's own flattened output,
// from the base ref) against t's live environment. Reuses kubectl's
// own exit-code-gated shape exactly — see doc.go for why this is not
// a literal "kubectl diff -k" call against the (already-flattened)
// rendering.
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
