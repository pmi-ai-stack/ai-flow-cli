package helm

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// resource is one parsed manifest document — identical shape to
// kubectl's own (.ai/phase-26/plan.md §0.11: duplicated, not shared,
// across the three Kubernetes-family addon packages).
type resource struct {
	apiVersion string
	kind       string
	namespace  string
	name       string
	doc        map[string]any
	file       string
}

func buildAddress(namespace, kind, name string) string {
	if namespace == "" {
		namespace = "-"
	}
	return namespace + "/" + kind + "/" + name
}

func (r resource) address() string {
	return buildAddress(r.namespace, r.kind, r.name)
}

func isManifestFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// parseManifests reads every *.yaml/*.yml file directly in dir (never
// recursive) — this addon's own Render always writes exactly one such
// file ("rendered.yaml," helm template's own stdout).
func parseManifests(dir string) ([]resource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("helm: read %s: %w", dir, err)
	}
	var resources []resource
	for _, e := range entries {
		if e.IsDir() || !isManifestFile(e.Name()) {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: path comes from os.ReadDir over a directory this package itself resolved
		if readErr != nil {
			return nil, fmt.Errorf("helm: read %s: %w", e.Name(), readErr)
		}
		found, parseErr := parseManifestBytes(data, e.Name())
		if parseErr != nil {
			return nil, parseErr
		}
		resources = append(resources, found...)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].address() < resources[j].address() })
	return resources, nil
}

// parseManifestBytes decodes every document in data, keeping only the
// ones shaped like a manifest (non-empty apiVersion and kind) — a
// bytes.Reader variant of kubectl's own file-based parser, needed here
// because "helm get manifest"'s own stdout is never written to a file
// before this package parses it (unlike every other addon's own
// declared-manifest source, which always starts life as a real file
// on disk). A malformed document stops decoding and returns whatever
// was already found, with no error — a broken rendering is Validate's
// job to report, not this function's to fail the whole call on.
func parseManifestBytes(data []byte, sourceLabel string) ([]resource, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var out []resource
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			break // io.EOF or a decode error — both stop here
		}
		if doc == nil {
			continue
		}
		apiVersion, _ := doc["apiVersion"].(string)
		kind, _ := doc["kind"].(string)
		if apiVersion == "" || kind == "" {
			continue
		}
		var namespace, name string
		if meta, ok := doc["metadata"].(map[string]any); ok {
			namespace, _ = meta["namespace"].(string)
			name, _ = meta["name"].(string)
		}
		out = append(out, resource{apiVersion: apiVersion, kind: kind, namespace: namespace, name: name, doc: doc, file: sourceLabel})
	}
	return out, nil
}
