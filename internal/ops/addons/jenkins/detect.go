package jenkins

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// Detect walks repo's checkout for every file exactly named
// "Jenkinsfile" (any depth), every "jobs/*.groovy" (Job DSL) and every
// "jobs/*.xml" (job-config) — one ops.Target per file found, Path
// "/"-separated relative to repo.LocalPath.
func (a *Addon) Detect(repo detect.Repo, changed []string) ([]ops.Target, error) {
	var found []string
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
		if isJenkinsFile(repo.LocalPath, path) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("jenkins: detect: %w", err)
	}
	sort.Strings(found)

	targets := make([]ops.Target, 0, len(found))
	for _, path := range found {
		rel, err := filepath.Rel(repo.LocalPath, path)
		if err != nil {
			return nil, fmt.Errorf("jenkins: detect: %w", err)
		}
		p := filepath.ToSlash(rel)
		targets = append(targets, ops.Target{Addon: "jenkins", Path: p, Files: ops.FilesUnderPath(p, changed)})
	}
	return targets, nil
}

// isJenkinsFile reports whether path (under root) is one of this
// addon's three Detect signals.
func isJenkinsFile(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	switch {
	case base == "Jenkinsfile":
		return true
	case strings.HasPrefix(rel, "jobs/") && strings.HasSuffix(base, ".groovy"):
		return true
	case strings.HasPrefix(rel, "jobs/") && strings.HasSuffix(base, ".xml"):
		return true
	default:
		return false
	}
}
