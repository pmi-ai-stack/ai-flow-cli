package kustomize

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// e2eHarnessFiles are the suite-config filenames Harnesses detects —
// identical to kubectl's own ("harnesses as kubectl," per the spec).
var e2eHarnessFiles = map[string]bool{"kuttl-test.yaml": true, "chainsaw-test.yaml": true}

// Harnesses lists every kuttl-test.yaml/chainsaw-test.yaml file under
// t.Path — detection and naming only, identical to kubectl's own.
// internal/ops/harness.ToolFor already dispatches these two file
// names for any addon (its own doc comment names this phase by
// number), so no change to that package is needed here.
func (a *Addon) Harnesses(repo detect.Repo, t ops.Target) ([]ops.Harness, error) {
	root := filepath.Join(repo.LocalPath, filepath.FromSlash(t.Path))
	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if e2eHarnessFiles[d.Name()] {
			rel, relErr := filepath.Rel(repo.LocalPath, path)
			if relErr != nil {
				return relErr
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("kustomize: harnesses: %w", err)
	}
	sort.Strings(names)

	harnesses := make([]ops.Harness, 0, len(names))
	for _, n := range names {
		harnesses = append(harnesses, ops.Harness{Name: n, Kind: ops.HarnessE2E})
	}
	return harnesses, nil
}
