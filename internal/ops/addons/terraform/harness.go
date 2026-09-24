package terraform

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

// terratestImport is the one text signal terratestFiles looks for — a
// *_test.go file's own reference to Terratest's real, stable Go module
// path — the same "plain text scan, no library dependency" posture
// detect.go's own terraformBlock regex already uses for HCL, applied
// here to Go source instead of HCL.
const terratestImport = "github.com/gruntwork-io/terratest"

// Harnesses lists every "*.tftest.hcl" file under t.Path (terraform
// test's own native file extension) as a Kind: HarnessUnit harness,
// and — phase 24's own addition — every "*_test.go" file under t.Path
// that imports Terratest as a Kind: HarnessE2E harness (the capability
// matrix's own "terraform test / Terratest on the sandbox workspace"
// E2e-harness cell). Detection and naming only; running one is
// internal/ops/harness's own job (phase 24), never this method — it
// calls no subprocess and needs no allowlist row of its own, keeping
// "terraform"/"go" out of TestEphemeralRowsReachableOnlyFromDesignatedCallers's
// allowed list for this file specifically.
//
// Disclosed limitation (the phase-24 plan's design decision 3): the
// conventional Terratest layout puts tests in a sibling top-level
// test/ directory *outside* the module's own target path (referencing
// the module via "../../modules/x"), which this scan — scoped to
// t.Path exactly like the .tftest.hcl scan beside it — does not find.
// Only a Terratest file that happens to live inside the target's own
// directory tree is detected today.
func (a *Addon) Harnesses(repo detect.Repo, t ops.Target) ([]ops.Harness, error) {
	root := filepath.Join(repo.LocalPath, filepath.FromSlash(t.Path))
	var unitNames, e2eNames []string
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

		switch {
		case strings.HasSuffix(d.Name(), ".tftest.hcl"):
			rel, relErr := filepath.Rel(repo.LocalPath, path)
			if relErr != nil {
				return relErr
			}
			unitNames = append(unitNames, filepath.ToSlash(rel))
		case strings.HasSuffix(d.Name(), "_test.go"):
			isTerratest, readErr := fileContains(path, terratestImport)
			if readErr != nil {
				return readErr
			}
			if isTerratest {
				rel, relErr := filepath.Rel(repo.LocalPath, path)
				if relErr != nil {
					return relErr
				}
				e2eNames = append(e2eNames, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("terraform: harnesses: %w", err)
	}
	sort.Strings(unitNames)
	sort.Strings(e2eNames)

	harnesses := make([]ops.Harness, 0, len(unitNames)+len(e2eNames))
	for _, n := range unitNames {
		harnesses = append(harnesses, ops.Harness{Name: n, Kind: ops.HarnessUnit})
	}
	for _, n := range e2eNames {
		harnesses = append(harnesses, ops.Harness{Name: n, Kind: ops.HarnessE2E})
	}
	return harnesses, nil
}

// fileContains reports whether path's own content contains substr —
// used to recognize a Terratest file by its own import, without
// parsing Go source.
func fileContains(path, substr string) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from filepath.WalkDir over this addon's own already-resolved target directory, not external input
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), substr), nil
}
