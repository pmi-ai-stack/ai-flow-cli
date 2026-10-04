package terraform

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

// Render materializes t (its call-scoped Path already resolved to an
// absolute source directory — the convention fixtureaddon.Render and
// internal/ops/render.renderToCache both already establish) into out:
// a recursive copy of t.Path's own subtree (skipping .terraform/ — a
// module whose local module sources escape t.Path via "../" is a
// known, documented limitation, the phase-21 plan's own §1.2.1, not
// silently handled), then a real "terraform init -backend=false" and
// "terraform validate" inside out, both against
// TF_PLUGIN_CACHE_DIR=<home>/cache/terraform (phase 19's own
// plumbing-with-no-caller, first real caller here). Returns
// Marked: nil always — see the plan's design decision on why (§0.5):
// Render never resolves a live value, so it has nothing real to mark;
// Action.Sensitive (DryRun's own job) carries the tool's sensitivity
// signal instead.
func (a *Addon) Render(ctx context.Context, t ops.Target, _ ops.Ref, out string) (*ops.Rendered, error) {
	if err := copyTree(t.Path, out); err != nil {
		return nil, fmt.Errorf("terraform: render: %w", err)
	}

	opts := exec.RunOpts{Dir: out, Env: a.tfEnv(), Timeout: a.timeout()}
	if _, err := a.x.Run(ctx, "terraform", []string{"init", "-backend=false", "-input=false", "-no-color"}, opts); err != nil {
		return nil, err
	}
	if _, err := a.x.Run(ctx, "terraform", []string{"validate", "-no-color"}, opts); err != nil {
		return nil, err
	}
	return &ops.Rendered{Dir: out}, nil
}

// tfEnv is every real terraform subprocess's own extra environment —
// just TF_PLUGIN_CACHE_DIR; AWS_*/ARM_*/GOOGLE_APPLICATION_CREDENTIALS
// (the spec's own "Secrets and live access" table) are inherited from
// the ambient process environment exec.Runner.Run already passes
// through (opts.Env is appended, never a replacement).
func (a *Addon) tfEnv() []string {
	return []string{"TF_PLUGIN_CACHE_DIR=" + render.TerraformPluginCacheDir(a.home)}
}

func (a *Addon) timeout() time.Duration {
	return a.cfg.Ops.ToolTimeout()
}

// copyTree recursively copies src's own regular files into dst,
// skipping any skipDir directory (in particular .terraform/ — a prior
// init's own cache must never be carried into a fresh rendering).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if rel != "." && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o700)
		}
		if !ops.IsTreeInput(d.Type()) {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from WalkDir over src, a source directory this package itself resolved
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600) //nolint:gosec // G306: 0600 matches fixtureaddon.Render's own file mode for a rendering
	})
}
