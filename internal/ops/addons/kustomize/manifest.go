package kustomize

import (
	"bytes"
	"fmt"
	"io"
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

// buildAddress assembles "<namespace or "-">/<kind>/<name>".
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
// file ("rendered.yaml," kustomize build's own stdout), so this is
// what both Drift and DryRun parse the "declared" side from.
func parseManifests(dir string) ([]resource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("kustomize: read %s: %w", dir, err)
	}
	return parseManifestsFromEntries(dir, entries)
}

func parseManifestsFromEntries(dir string, entries []os.DirEntry) ([]resource, error) {
	var resources []resource
	for _, e := range entries {
		if e.IsDir() || !isManifestFile(e.Name()) {
			continue
		}
		found, err := parseManifestFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		resources = append(resources, found...)
	}

	sort.Slice(resources, func(i, j int) bool { return resources[i].address() < resources[j].address() })
	return resources, nil
}

// parseManifestFile decodes every document in path, keeping only the
// ones shaped like a manifest (non-empty apiVersion and kind). A
// malformed document stops decoding *this file* and returns whatever
// was already found, with no error — a broken rendering is
// kubeconform's (Validate's) job to report, not this function's to
// fail the whole call on.
func parseManifestFile(path string) ([]resource, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path comes from os.ReadDir over a directory this package itself resolved
	if err != nil {
		return nil, fmt.Errorf("kustomize: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return decodeManifests(f, filepath.Base(path)), nil
}

// parseManifestBytes is parseManifestFile's own decode loop over
// already-in-memory content — used by Render to parse a subprocess's
// own stdout directly, without a redundant write-then-read-back round
// trip through the file it also writes to disk for Validate's later
// use.
func parseManifestBytes(data []byte, sourceLabel string) []resource {
	return decodeManifests(bytes.NewReader(data), sourceLabel)
}

// decodeManifests is parseManifestFile's/parseManifestBytes's shared
// decode loop, keeping only documents shaped like a manifest
// (non-empty apiVersion and kind). A malformed document stops decoding
// *this source* and returns whatever was already found, with no error —
// a broken rendering is kubeconform's (Validate's) job to report, not
// this function's to fail the whole call on.
func decodeManifests(r io.Reader, sourceLabel string) []resource {
	dec := yaml.NewDecoder(r)
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
	return out
}
