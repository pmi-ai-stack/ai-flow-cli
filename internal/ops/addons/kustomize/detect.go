package kustomize

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// kustomizationFile is Detect's own literal signal — "kustomization.yaml".
const kustomizationFile = "kustomization.yaml"

// skipDirName reports whether name is never descended into: .git and
// any other dot-directory.
func skipDirName(name string) bool {
	return strings.HasPrefix(name, ".")
}

// Detect walks repo's checkout for every directory directly containing
// a kustomization.yaml — one ops.Target per directory found. Unlike
// kubectl's own Detect, a found overlay directory's own subtree is
// never scanned for a further, nested target: an overlay's own
// bases/components references point *outside* itself via "../" (the
// spec's own kustomize convention — see .ai/phase-26/plan.md §0's own
// verified kustomize build ../../base example), so there is no
// legitimate "nested overlay inside an overlay" case the way kubectl's
// raw-manifests directories can legitimately nest.
func (a *Addon) Detect(repo detect.Repo, changed []string) ([]ops.Target, error) {
	var dirs []string
	err := filepath.WalkDir(repo.LocalPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != repo.LocalPath && skipDirName(d.Name()) {
			return fs.SkipDir
		}
		if hasKustomization(path) {
			dirs = append(dirs, path)
			return fs.SkipDir // this overlay's own subtree is its own content, never a nested target
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("kustomize: detect: %w", err)
	}
	sort.Strings(dirs)

	targets := make([]ops.Target, 0, len(dirs))
	for _, dir := range dirs {
		rel, relErr := filepath.Rel(repo.LocalPath, dir)
		if relErr != nil {
			return nil, fmt.Errorf("kustomize: detect: %w", relErr)
		}
		p := filepath.ToSlash(rel)
		targets = append(targets, ops.Target{Addon: "kustomize", Path: p, Files: ops.FilesUnderPath(p, changed)})
	}
	return targets, nil
}

func hasKustomization(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, kustomizationFile))
	return err == nil && !info.IsDir()
}
