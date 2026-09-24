package helm

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

func skipDirName(name string) bool {
	return strings.HasPrefix(name, ".")
}

// Detect walks repo's checkout for every directory directly containing
// a Chart.yaml — one ops.Target per directory found. Does not descend
// beneath a found chart directory: a chart's own charts/ (subcharts)
// and templates/ are its own content, never a second, independent
// target (mirroring terraform's own flat, single-level root-module
// Detect). A chart with no values.yaml is still a valid target
// (.ai/phase-26/plan.md §2 design decision 2).
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
		if hasChartFile(path) {
			dirs = append(dirs, path)
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("helm: detect: %w", err)
	}
	sort.Strings(dirs)

	targets := make([]ops.Target, 0, len(dirs))
	for _, dir := range dirs {
		rel, relErr := filepath.Rel(repo.LocalPath, dir)
		if relErr != nil {
			return nil, fmt.Errorf("helm: detect: %w", relErr)
		}
		p := filepath.ToSlash(rel)
		targets = append(targets, ops.Target{Addon: "helm", Path: p, Files: ops.FilesUnderPath(p, changed)})
	}
	return targets, nil
}

func hasChartFile(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, chartFileName))
	return err == nil && !info.IsDir()
}
