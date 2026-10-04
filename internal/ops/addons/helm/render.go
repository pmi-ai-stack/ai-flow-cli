package helm

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

const renderedFileName = "rendered.yaml"

// chartCopyDirName is the second thing Render writes — a plain copy
// of the chart source, so Validate (which receives only a *Rendered
// whose Dir is the real cache directory, never the pre-render source —
// see doc.go) can still reach helm lint's own target. See
// .ai/phase-26/plan.md §1.1.13/§1.1.14.
const chartCopyDirName = "chart"

const secretKind = "Secret"

// Render runs "helm template <name> <t.Path> [-f values.yaml]" and
// writes its stdout verbatim to <out>/rendered.yaml, copies t.Path
// itself into <out>/chart/, then collects every kind: Secret
// document's data/stringData values from rendered.yaml as
// redact.MarkedValue entries — values are already substituted by helm
// template itself, so this pass sees literal scalars exactly like
// kubectl's own already does.
func (a *Addon) Render(ctx context.Context, t ops.Target, _ ops.Ref, out string) (*ops.Rendered, error) {
	meta, err := readChartMeta(t.Path)
	if err != nil {
		return nil, err
	}

	args := []string{"template", meta.Name, t.Path}
	if vf, ok := findValuesFile(t.Path); ok {
		args = append(args, "-f", vf)
	}
	templateOut, err := a.x.Run(ctx, "helm", args, exec.RunOpts{Timeout: a.timeout()})
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(out, 0o700); err != nil {
		return nil, fmt.Errorf("helm: render: %w", err)
	}
	renderedPath := filepath.Join(out, renderedFileName)
	if err := os.WriteFile(renderedPath, []byte(templateOut.Stdout), 0o600); err != nil {
		return nil, fmt.Errorf("helm: render: %w", err)
	}

	if err := copyTree(t.Path, filepath.Join(out, chartCopyDirName)); err != nil {
		return nil, fmt.Errorf("helm: render: %w", err)
	}

	marked, err := collectSecretMarkings([]byte(templateOut.Stdout))
	if err != nil {
		return nil, fmt.Errorf("helm: render: %w", err)
	}
	return &ops.Rendered{Dir: out, Marked: marked}, nil
}

// copyTree recursively copies src's own regular files into dst,
// skipping any dot-directory — identical to kubectl's own copyTree.
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
			if rel != "." && skipDirName(d.Name()) {
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
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600) //nolint:gosec // G306: 0600 matches every other addon's own Render file mode
	})
}

// collectSecretMarkings parses data (already-in-memory rendered.yaml
// content, never re-read off disk — never the chart/ copy either,
// which still carries template syntax, not real values) for every
// kind: Secret document's data/stringData keys.
func collectSecretMarkings(data []byte) ([]redact.MarkedValue, error) {
	resources, err := parseManifestBytes(data, renderedFileName)
	if err != nil {
		return nil, err
	}

	var marked []redact.MarkedValue
	for _, r := range resources {
		if r.kind != secretKind {
			continue
		}
		marked = append(marked, secretMarkings(r)...)
	}
	return marked, nil
}

func secretMarkings(r resource) []redact.MarkedValue {
	var marked []redact.MarkedValue
	for _, field := range []string{"data", "stringData"} {
		m, ok := r.doc[field].(map[string]any)
		if !ok {
			continue
		}
		for key, v := range m {
			s, ok := v.(string)
			if !ok || s == "" {
				continue
			}
			marked = append(marked, redact.MarkedValue{
				Pointer: fmt.Sprintf("%s#/%s/%s/%s", r.file, field, r.name, key),
				Value:   s,
			})
		}
	}
	return marked
}
