package kubectl

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// secretKind is the one Kubernetes kind whose fields Render marks
// sensitive — a Secret's data/stringData values are already present,
// verbatim, in the rendered manifest text at Render time (unlike
// terraform, which never resolves a live value until a plan exists —
// Determinism §6's own worked example is exactly this case).
const secretKind = "Secret"

// Render materializes t (its call-scoped Path already resolved to an
// absolute source directory) into out: a plain recursive file copy —
// the capability matrix's own Render cell is just "copy," no
// subprocess at all, unlike terraform's own init+validate — followed
// by a second pass over the copied files collecting every Secret's
// data/stringData values as redact.MarkedValue entries. A file that
// fails to parse during the marking pass is skipped, not a Render
// failure: Render's only documented job is "copy"; a malformed
// manifest is Validate's (kubeconform's) job to report, not this
// method's to fail on.
func (a *Addon) Render(_ context.Context, t ops.Target, _ ops.Ref, out string) (*ops.Rendered, error) {
	if err := copyTree(t.Path, out); err != nil {
		return nil, fmt.Errorf("kubectl: render: %w", err)
	}
	marked, err := collectSecretMarkings(out)
	if err != nil {
		return nil, fmt.Errorf("kubectl: render: %w", err)
	}
	return &ops.Rendered{Dir: out, Marked: marked}, nil
}

// copyTree recursively copies src's own regular files into dst,
// skipping any skipDirName directory — the same convention terraform's
// own copyTree already established for its addon package.
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
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600) //nolint:gosec // G306: 0600 matches terraform's own Render file mode for a rendering
	})
}

// collectSecretMarkings walks dir's own directly-contained manifest
// files (parseManifests's own non-recursive scope — a rendering can
// hold nested subdirectories the target itself never claimed as
// manifests, e.g. docs; only the target's own directory-level
// manifests are ever marked) for every kind: Secret document's
// data/stringData keys, returning one redact.MarkedValue per key whose
// value is a string. A malformed manifest file contributes no
// resources (parseManifests's own tolerant behavior — Render's doc
// comment explains why) rather than failing this call.
func collectSecretMarkings(dir string) ([]redact.MarkedValue, error) {
	resources, err := parseManifests(dir)
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
