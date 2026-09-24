package githubactions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// RenderedFile is the fixed basename every rendering carries its one
// workflow file under — internal/ops/render.Render rewrites
// Target.Path to the target's own absolute source *file* path before
// calling this method (the phase-19 convention every addon relies on
// for its own Target.Path), so Render copies that single file's own
// bytes to a fixed, addon-chosen name inside out rather than trying to
// recreate an absolute path's own directory structure underneath it.
// Validate/DryRun/Harnesses all reference this same constant.
const RenderedFile = "workflow.yml"

// Render materializes t into out: a plain byte copy of t's own single
// workflow file to RenderedFile, no subprocess at all (the capability
// matrix's own literal cell: "copy"). Marked is always nil — a
// workflow file has no addon-recognized sensitive-value convention (a
// "${{ secrets.X }}" expression names a secret, it never contains
// one).
func (a *Addon) Render(_ context.Context, t ops.Target, _ ops.Ref, out string) (*ops.Rendered, error) {
	data, err := os.ReadFile(t.Path)
	if err != nil {
		return nil, fmt.Errorf("github-actions: render: %w", err)
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		return nil, fmt.Errorf("github-actions: render: %w", err)
	}
	dst := filepath.Join(out, RenderedFile)
	if err := os.WriteFile(dst, data, 0o600); err != nil { //nolint:gosec // G306: 0600 matches every other addon's own rendering file mode
		return nil, fmt.Errorf("github-actions: render: %w", err)
	}
	return &ops.Rendered{Dir: out}, nil
}
