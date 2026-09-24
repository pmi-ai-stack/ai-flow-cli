package kustomize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// renderedFileName is the one file every Render call writes — a
// kustomize target is always flattened into a single plain-YAML
// rendering (kustomize build's own stdout), the same "one directory,
// parseable by parseManifests" shape every other addon's rendering
// takes. See doc.go for why Drift/DryRun never re-target this file
// with a "-k" flag.
const renderedFileName = "rendered.yaml"

const secretKind = "Secret"

// Render runs "kustomize build <t.Path>" and writes its stdout
// verbatim to <out>/rendered.yaml, then collects every kind: Secret
// document's data/stringData values as redact.MarkedValue entries —
// identical secret-marking pass to kubectl's own (values are already
// substituted by kustomize build itself, so a marked value's literal
// text is exactly what a later redaction needs to find).
func (a *Addon) Render(ctx context.Context, t ops.Target, _ ops.Ref, out string) (*ops.Rendered, error) {
	buildOut, err := a.x.Run(ctx, "kustomize", []string{"build", t.Path}, exec.RunOpts{Timeout: a.timeout()})
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(out, 0o700); err != nil {
		return nil, fmt.Errorf("kustomize: render: %w", err)
	}
	renderedPath := filepath.Join(out, renderedFileName)
	if err := os.WriteFile(renderedPath, []byte(buildOut.Stdout), 0o600); err != nil {
		return nil, fmt.Errorf("kustomize: render: %w", err)
	}

	marked := collectSecretMarkings([]byte(buildOut.Stdout), renderedFileName)
	return &ops.Rendered{Dir: out, Marked: marked}, nil
}

func collectSecretMarkings(data []byte, sourceLabel string) []redact.MarkedValue {
	resources := parseManifestBytes(data, sourceLabel)

	var marked []redact.MarkedValue
	for _, r := range resources {
		if r.kind != secretKind {
			continue
		}
		marked = append(marked, secretMarkings(r)...)
	}
	return marked
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
