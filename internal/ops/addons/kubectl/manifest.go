package kubectl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// resource is one parsed manifest document — apiVersion+kind is
// Detect's own signal (a document missing either is not a manifest,
// skipped by parseManifests); doc is the full decoded document (a
// map[string]interface{} — yaml.v3 decodes mappings this way for an
// `any` target, unlike yaml.v2's map[interface{}]interface{} — so it
// is already JSON-marshalable, no conversion pass needed by
// normalize.go's decodeJSONStream/topLevelDiff).
type resource struct {
	apiVersion string
	kind       string
	namespace  string
	name       string
	doc        map[string]any
	file       string
}

// buildAddress is the one place "<namespace or "-">/<kind>/<name>" is
// assembled — resource.address() (a parsed manifest) and drift.go's
// objectAddress (a decoded kubectl JSON object) both call this rather
// than each re-deriving the same "-" fallback-for-empty-namespace
// convention independently.
func buildAddress(namespace, kind, name string) string {
	if namespace == "" {
		namespace = "-"
	}
	return namespace + "/" + kind + "/" + name
}

// address is this resource's own stable, human-readable identity —
// used as ops.Action.Address/Name and as render-drift-results.json's
// own drift_resources[] entries, the same role terraform's own
// resource address string already plays for that addon.
func (r resource) address() string {
	return buildAddress(r.namespace, r.kind, r.name)
}

// isManifestFile reports whether name has a YAML extension —
// parseManifests's own file filter.
func isManifestFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// parseManifests reads every *.yaml/*.yml file directly in dir (never
// recursive — a Target's own grain is one directory, the spec's own
// "a directory of raw manifests," mirroring terraform's per-directory,
// never-per-subtree root-module scope), splitting multi-document files
// via a yaml.Decoder loop, keeping only documents with a non-empty
// string apiVersion and kind. Sorted by address for determinism.
func parseManifests(dir string) ([]resource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("kubectl: read %s: %w", dir, err)
	}
	return parseManifestsFromEntries(dir, entries)
}

// parseManifestsFromEntries is parseManifests's own body, taking an
// already-fetched directory listing — detect.go's detectWalk has
// already called os.ReadDir(dir) once to check for a chart/kustomize
// marker file, so it calls this directly rather than paying for a
// second, redundant listing of the same directory.
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
// ones shaped like a manifest (non-empty apiVersion and kind). A blank
// document (a trailing "---" with nothing after it) decodes as a nil
// map and is silently skipped, not an error. A malformed document
// (invalid YAML) stops decoding *this file* and returns whatever
// manifests were already found in it, with no error: a broken file is
// kubeconform's (Validate's) job to report as a finding, not
// parseManifests's to fail the whole Detect/Drift/DryRun call on — a
// real I/O failure opening or reading the file is still a real error,
// since that is not "malformed content," it is this package's own
// filesystem access failing.
func parseManifestFile(path string) ([]resource, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path comes from os.ReadDir over a directory this package itself resolved
	if err != nil {
		return nil, fmt.Errorf("kubectl: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	return decodeManifests(f, filepath.Base(path)), nil
}

// decodeManifests is parseManifestFile's decode loop over an
// already-open reader, shared with detect.go's dirHasManifest, which
// has the file's bytes in memory already. file is the resource.file
// value every decoded resource carries.
func decodeManifests(r io.Reader, file string) []resource {
	dec := yaml.NewDecoder(r)
	var out []resource
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			break // io.EOF (clean end) or a decode error (malformed content) — both stop here, see parseManifestFile's doc comment
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
		out = append(out, resource{apiVersion: apiVersion, kind: kind, namespace: namespace, name: name, doc: doc, file: file})
	}
	return out
}
