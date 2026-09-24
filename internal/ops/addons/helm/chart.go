package helm

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// chartMeta is the subset of Chart.yaml's own shape this package
// needs — never a Kubernetes manifest, so it gets its own small typed
// struct rather than manifest.go's generic map[string]any resource.
type chartMeta struct {
	APIVersion string `yaml:"apiVersion"`
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	AppVersion string `yaml:"appVersion"`
}

const chartFileName = "Chart.yaml"

// readChartMeta parses dir's own Chart.yaml. Every subprocess this
// package calls that needs a Helm release name (get manifest, diff
// upgrade, history, rollback) reads it fresh via this function rather
// than storing it anywhere — see .ai/phase-26/plan.md §2 design
// decision 4 for why.
func readChartMeta(dir string) (chartMeta, error) {
	data, err := os.ReadFile(filepath.Join(dir, chartFileName)) //nolint:gosec // G304: dir is always a target this addon's own Detect already resolved
	if err != nil {
		return chartMeta{}, fmt.Errorf("helm: read %s: %w", chartFileName, err)
	}
	var m chartMeta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return chartMeta{}, fmt.Errorf("helm: parse %s: %w", chartFileName, err)
	}
	return m, nil
}

const valuesFileName = "values.yaml"

// findValuesFile reports dir's own values.yaml, if present. Its
// absence does not disqualify a directory from being a valid chart
// (Detect's own rule, .ai/phase-26/plan.md §2 design decision 2) —
// helm template/lint both run correctly with no -f flag at all.
func findValuesFile(dir string) (string, bool) {
	path := filepath.Join(dir, valuesFileName)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}
