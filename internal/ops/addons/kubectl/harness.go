package kubectl

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// e2eHarnessFiles are the suite-config filenames Harnesses detects —
// kuttl's own "kuttl-test.yaml" and chainsaw's own "chainsaw-test.yaml"
// — the capability matrix's own E2e-harness cell: "kuttl / chainsaw on
// kind." No Unit harness exists for kubectl (the matrix's own cell is
// "—" — Capabilities().UnitTest is false), so every Harness this
// method returns is Kind: HarnessE2E.
var e2eHarnessFiles = map[string]bool{"kuttl-test.yaml": true, "chainsaw-test.yaml": true}

// Harnesses lists every kuttl-test.yaml/chainsaw-test.yaml file under
// t.Path — detection and naming only; running one is phase 24's
// internal/ops/harness, never this method, so it calls no subprocess
// and needs no allowlist row (mirroring terraform's own harness.go
// exactly — .ai/phase-22/plan.md §0.3).
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
		return nil, fmt.Errorf("kubectl: harnesses: %w", err)
	}
	sort.Strings(names)

	harnesses := make([]ops.Harness, 0, len(names))
	for _, n := range names {
		harnesses = append(harnesses, ops.Harness{Name: n, Kind: ops.HarnessE2E})
	}
	return harnesses, nil
}
