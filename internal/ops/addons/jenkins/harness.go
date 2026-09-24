package jenkins

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

// jenkinsPipelineUnitImport is the one text signal this addon looks
// for — JenkinsPipelineUnit's own real, stable Maven coordinate
// (com.lesfurets.jenkins.unit) — the same "recognize by its own known
// import, no library dependency" convention
// terraform.Harnesses'/terratestImport's own constant already
// establishes.
const jenkinsPipelineUnitImport = "com.lesfurets.jenkins.unit"

// Harnesses is detection-only (doc.go/§0.8 of the phase-27 plan): any
// "*.groovy" file anywhere in repo importing JenkinsPipelineUnit
// becomes one Kind: HarnessUnit entry; t itself always becomes one
// Kind: HarnessE2E entry ("jenkinsfile-runner", the matrix's own
// opt-in cell — every Jenkins target is trivially runnable by it in
// principle, so this addon reports the signal unconditionally rather
// than guessing at a repository-specific opt-in marker the spec names
// but does not shape). Calls no subprocess.
func (a *Addon) Harnesses(repo detect.Repo, t ops.Target) ([]ops.Harness, error) {
	var unitNames []string
	err := filepath.WalkDir(repo.LocalPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != repo.LocalPath && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".groovy") {
			return nil
		}
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: path comes from WalkDir over this addon's own already-resolved repository checkout
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), jenkinsPipelineUnitImport) {
			rel, relErr := filepath.Rel(repo.LocalPath, path)
			if relErr != nil {
				return relErr
			}
			unitNames = append(unitNames, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("jenkins: harnesses: %w", err)
	}
	sort.Strings(unitNames)

	harnesses := make([]ops.Harness, 0, len(unitNames)+1)
	for _, n := range unitNames {
		harnesses = append(harnesses, ops.Harness{Name: n, Kind: ops.HarnessUnit})
	}
	harnesses = append(harnesses, ops.Harness{Name: t.Path + "#jenkinsfile-runner", Kind: ops.HarnessE2E})
	return harnesses, nil
}
