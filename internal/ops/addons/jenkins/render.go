package jenkins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Render materializes t into out: a plain byte copy of t's own single
// file to its own original base name (not a fixed generic name, unlike
// githubactions' own RenderedFile) — the phase-27 plan's own §0.7:
// internal/standards/check's "policy"/"scan" runners (conftest,
// zizmor) select their own parser by file extension, and this addon's
// own three target kinds (a bare "Jenkinsfile", "*.groovy", "*.xml")
// genuinely differ in what that parser needs to see, unlike
// github-actions' own workflow files, which are always ".yml"-shaped
// regardless of their original name. Marked is always nil — no
// addon-recognized sensitive-value convention exists for any of the
// three shapes.
func (a *Addon) Render(_ context.Context, t ops.Target, _ ops.Ref, out string) (*ops.Rendered, error) {
	data, err := os.ReadFile(t.Path)
	if err != nil {
		return nil, fmt.Errorf("jenkins: render: %w", err)
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		return nil, fmt.Errorf("jenkins: render: %w", err)
	}
	dst := filepath.Join(out, filepath.Base(t.Path))
	if err := os.WriteFile(dst, data, 0o600); err != nil { //nolint:gosec // G306: 0600 matches every other addon's own rendering file mode
		return nil, fmt.Errorf("jenkins: render: %w", err)
	}
	return &ops.Rendered{Dir: out}, nil
}

// soleFile returns the one regular file a rendering directory holds —
// every Render call in this package writes exactly one, so
// Validate/Drift/DryRun (which receive only *ops.Rendered, never the
// Target that named the file) can recover its own name this way rather
// than assuming a fixed one.
func soleFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("jenkins: read rendering %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("jenkins: rendering %s has no file", dir)
}

// readRendering resolves dir's own sole file (soleFile) and returns
// its content — Drift and DryRun both need exactly this pair before
// their own comparison, differing only in what they compare it
// against (baseRender vs. the branch rendering).
func readRendering(dir string) (content string, err error) {
	name, err := soleFile(dir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: dir is this addon's own rendering directory
	if err != nil {
		return "", fmt.Errorf("jenkins: read rendering %s: %w", dir, err)
	}
	return string(data), nil
}
