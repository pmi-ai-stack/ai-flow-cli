package terraform

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Harnesses lists every "*.tftest.hcl" file under t.Path (terraform
// test's own native file extension) as a Kind: HarnessUnit harness —
// detection and naming only; running one is phase 24's
// internal/ops/harness, never this method (so it calls no subprocess
// and needs no allowlist row of its own, keeping "terraform" out of
// TestEphemeralRowsReachableOnlyFromDesignatedCallers's allowed list
// for this file specifically). Terratest detection is not attempted —
// the phase-21 plan's own §1.2.6: phase 24's own plan bullet names
// Terratest as its own line item, alongside the harness execution
// engine that does not exist yet.
func (a *Addon) Harnesses(repo detect.Repo, t ops.Target) ([]ops.Harness, error) {
	root := filepath.Join(repo.LocalPath, filepath.FromSlash(t.Path))
	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".tftest.hcl") {
			rel, relErr := filepath.Rel(repo.LocalPath, path)
			if relErr != nil {
				return relErr
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("terraform: harnesses: %w", err)
	}
	sort.Strings(names)

	harnesses := make([]ops.Harness, 0, len(names))
	for _, n := range names {
		harnesses = append(harnesses, ops.Harness{Name: n, Kind: ops.HarnessUnit})
	}
	return harnesses, nil
}
