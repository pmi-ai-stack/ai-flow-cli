package githubactions

import (
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// workflowJobs reads path (a rendered workflow file) and returns its
// own top-level "jobs:" map, keyed by job id. A missing file reports
// an empty map, not an error.
func workflowJobs(path string) (map[string]map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is this addon's own rendering file, never external input
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]map[string]any{}, nil
		}
		return nil, err
	}

	var doc struct {
		Jobs map[string]map[string]any `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Jobs == nil {
		doc.Jobs = map[string]map[string]any{}
	}
	return doc.Jobs, nil
}

// sortedJobIDs returns jobs' own keys, sorted — DryRun and Harnesses
// both need the same deterministic job-id ordering.
func sortedJobIDs(jobs map[string]map[string]any) []string {
	ids := make([]string, 0, len(jobs))
	for id := range jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
