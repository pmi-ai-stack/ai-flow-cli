package kubectl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// chartOrKustomizeMarker names the files that make a directory
// Helm's/Kustomize's own, not this addon's — "outside charts and
// overlays" per the spec's own Detect column.
var chartOrKustomizeMarker = map[string]bool{"Chart.yaml": true, "kustomization.yaml": true}

// skipDirName reports whether name is never descended into: .git and
// any other dot-directory, the same convention terraform's own
// skipDir already established for its addon package.
func skipDirName(name string) bool {
	return strings.HasPrefix(name, ".")
}

// Detect walks repo's checkout for every directory whose own,
// directly-contained files parse as at least one manifest (Detect's
// own apiVersion+kind signal), skipping any directory that itself
// contains a chart/kustomize marker file — and everything beneath it —
// entirely.
//
// This is a hand-rolled, os.ReadDir-first recursive walk rather than
// filepath.WalkDir+SkipDir (terraform's own approach, phase 21):
// WalkDir invokes its callback for a directory's files and
// subdirectories interleaved in lexical order, so a subdirectory can
// already be recursed into before a marker file that should have
// excluded the whole directory is ever seen — this addon's own skip
// decision depends on a sibling *file's* content, not just a
// directory's own name the way terraform's ".terraform" skip does, so
// the full entry list must be read before any recursion decision is
// made (.ai/phase-22/plan.md's design decision 1).
func (a *Addon) Detect(repo detect.Repo, changed []string) ([]ops.Target, error) {
	var targets []ops.Target
	if err := detectWalk(repo.LocalPath, repo.LocalPath, changed, &targets); err != nil {
		return nil, fmt.Errorf("kubectl: detect: %w", err)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Path < targets[j].Path })
	return targets, nil
}

func detectWalk(root, dir string, changed []string, targets *[]ops.Target) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	for _, e := range entries {
		if !e.IsDir() && chartOrKustomizeMarker[e.Name()] {
			return nil // this directory (and everything under it) belongs to helm/kustomize, not this addon
		}
	}

	found, err := dirHasManifest(dir, entries)
	if err != nil {
		return err
	}
	if found {
		rel, relErr := filepath.Rel(root, dir)
		if relErr != nil {
			return relErr
		}
		p := filepath.ToSlash(rel)
		*targets = append(*targets, ops.Target{Addon: "kubectl", Path: p, Files: filesUnder(p, changed)})
	}

	for _, e := range entries {
		if e.IsDir() && !skipDirName(e.Name()) {
			if err := detectWalk(root, filepath.Join(dir, e.Name()), changed, targets); err != nil {
				return err
			}
		}
	}
	return nil
}

// apiVersionKey is the byte literal dirHasManifest scans for before
// paying for a YAML decode. A document without these bytes can never
// decode to a non-empty apiVersion string, so skipping it changes no
// result. Deliberately "apiVersion", not "apiVersion:" — a quoted key
// ("apiVersion": …, JSON-style YAML) must still be a candidate.
var apiVersionKey = []byte("apiVersion")

// dirHasManifest reports whether any YAML file directly in dir holds at
// least one manifest document — all Detect needs, where Drift/DryRun/
// Render need parseManifests's full, sorted resource list. Two
// shortcuts over parseManifestsFromEntries, both result-preserving:
// a file whose bytes never contain apiVersionKey (scanned in full, not
// a prefix, so a manifest after a long licence header or in a later
// "---" document is still found) is never YAML-decoded, and the scan
// stops at the first manifest. YAML decoding of every config file in a
// large repository was nearly all of DetectAll's cost.
func dirHasManifest(dir string, entries []os.DirEntry) (bool, error) {
	for _, e := range entries {
		if e.IsDir() || !isManifestFile(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from os.ReadDir over a directory this package itself resolved
		if err != nil {
			return false, fmt.Errorf("kubectl: read %s: %w", path, err)
		}
		if !bytes.Contains(data, apiVersionKey) {
			continue
		}
		if len(decodeManifests(bytes.NewReader(data), e.Name())) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// filesUnder returns every entry of changed whose own directory is
// exactly path — a *direct-child* match, not the prefix match
// fixtureaddon.Detect/terraform.Detect use for their own (always
// single-level, never independently re-claimed by a nested target)
// root-module targets. This addon's own targets can nest arbitrarily
// (a manifests directory's own subdirectory is a second, independent
// Target, §1.1.3), so a prefix match would let a parent directory's
// Files also claim a nested target's own files, double-counting the
// same changed file against two targets; a direct-child match gives
// each target exactly its own directly-contained changed files, no
// double-counting, and is symmetric with parseManifests's own
// non-recursive scope.
func filesUnder(path string, changed []string) []string {
	var files []string
	for _, f := range changed {
		if filepath.ToSlash(filepath.Dir(f)) == path {
			files = append(files, f)
		}
	}
	return files
}

// SkipTreeDir implements ops.TreeSkipper with the same skipDirName rule
// Render's copyTree applies, so render.TreeHash never hashes content
// the rendering cannot include.
func (a *Addon) SkipTreeDir(name string) bool {
	return skipDirName(name)
}
