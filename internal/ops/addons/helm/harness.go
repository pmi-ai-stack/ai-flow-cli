package helm

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Harnesses detects, but does not run, the capability matrix's own
// three helm-family tool signals — detection only, no subprocess,
// exactly like every other addon's own Harnesses:
//
//   - "helm unittest": every tests/*_test.yaml file directly under
//     t.Path (the helm-unittest plugin's own documented convention),
//     Kind: HarnessUnit.
//   - "ct lint": the chart itself is always ct-lintable by
//     construction (t.Path is a Target this addon's own Detect already
//     confirmed carries a Chart.yaml), so one HarnessUnit entry naming
//     the chart's own Chart.yaml is the signal, not a separate marker
//     file this phase invents.
//   - "ct install": every ci/*.yaml file directly under t.Path
//     (chart-testing's own "ct install --config ci/<case>-values.yaml"
//     convention), Kind: HarnessE2E.
//
// None of these three tool identities is yet a name
// internal/ops/harness.ToolFor recognizes — silently dropped by `ops
// test run` today, a disclosed, accepted gap
// (.ai/phase-26/plan.md §0.5/§1.2.3), not a regression this phase
// introduces.
func (a *Addon) Harnesses(repo detect.Repo, t ops.Target) ([]ops.Harness, error) {
	root := filepath.Join(repo.LocalPath, filepath.FromSlash(t.Path))

	var harnesses []ops.Harness
	harnesses = append(harnesses, ops.Harness{Name: filepath.ToSlash(filepath.Join(t.Path, chartFileName)), Kind: ops.HarnessUnit})

	unitTests, err := listFilesInDir(filepath.Join(root, "tests"), func(name string) bool {
		return strings.HasSuffix(name, "_test.yaml") || strings.HasSuffix(name, "_test.yml")
	})
	if err != nil {
		return nil, err
	}
	for _, rel := range unitTests {
		harnesses = append(harnesses, ops.Harness{Name: filepath.ToSlash(filepath.Join(t.Path, "tests", rel)), Kind: ops.HarnessUnit})
	}

	ciCases, err := listFilesInDir(filepath.Join(root, "ci"), isManifestFile)
	if err != nil {
		return nil, err
	}
	for _, rel := range ciCases {
		harnesses = append(harnesses, ops.Harness{Name: filepath.ToSlash(filepath.Join(t.Path, "ci", rel)), Kind: ops.HarnessE2E})
	}

	return harnesses, nil
}

// listFilesInDir returns the sorted, base names of every regular file
// directly in dir (never recursive) matching keep — a missing dir
// produces an empty, non-error result (a chart with no tests/ or no
// ci/ directory is the common case, not a failure).
func listFilesInDir(dir string, keep func(name string) bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && keep(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
