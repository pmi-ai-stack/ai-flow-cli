package githubactions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// workflowsDir is the one directory GitHub itself recognizes — the
// spec's own literal Detect signal, ".github/workflows/*.yml".
const workflowsDir = ".github" + string(filepath.Separator) + "workflows"

// Detect returns one ops.Target per file directly under
// repo's own .github/workflows/ (".yml" or ".yaml" — GitHub accepts
// both), Path "/"-separated relative to repo.LocalPath. Unlike every
// other addon, a Target here is one *file*, not a directory ("a
// workflow file," the spec's own singular Target example) — there is
// nothing beneath it to recurse into.
func (a *Addon) Detect(repo detect.Repo, changed []string) ([]ops.Target, error) {
	dir := filepath.Join(repo.LocalPath, workflowsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("github-actions: detect: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	targets := make([]ops.Target, 0, len(names))
	for _, name := range names {
		rel, err := filepath.Rel(repo.LocalPath, filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("github-actions: detect: %w", err)
		}
		p := filepath.ToSlash(rel)
		targets = append(targets, ops.Target{Addon: "github-actions", Path: p, Files: ops.FilesUnderPath(p, changed)})
	}
	return targets, nil
}
