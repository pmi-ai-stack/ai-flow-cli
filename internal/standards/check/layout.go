package check

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// layoutConfig mirrors packs/reference/languages/<lang>/checks/layout.json.
// test_placement/test_suffix are read but produce no finding —
// verifying "every source file has a sibling test" needs a
// source/test pairing heuristic internal/detect carries no primitive
// for; inventing one is a scope decision the phase-14 plan does not
// ask for (see the plan's design decision 12).
type layoutConfig struct {
	RequiredFiles  []string `json:"required_files"`
	TestPlacement  string   `json:"test_placement"`
	TestSuffix     []string `json:"test_suffix"`
	ForbiddenPaths []string `json:"forbidden_paths"`
}

// runLayout implements the "layout" kind: a built-in check reading
// layout.json against the repository's own file tree.
func runLayout(_ context.Context, in RunInput, _ exec.Execer, _ time.Duration) ([]Finding, error) {
	var cfg layoutConfig
	if err := readConfigJSON(in.Check, &cfg); err != nil {
		return nil, err
	}

	var findings []Finding
	for _, rf := range cfg.RequiredFiles {
		if _, err := os.Stat(filepath.Join(in.Repo.LocalPath, rf)); err != nil {
			findings = append(findings, layoutFinding(in, rf, fmt.Sprintf("required file %s is missing", rf)))
		}
	}

	matches, err := forbiddenPathMatches(in.Repo.LocalPath, cfg.ForbiddenPaths)
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		findings = append(findings, layoutFinding(in, m, fmt.Sprintf("%s matches a forbidden path", m)))
	}

	return findings, nil
}

func layoutFinding(in RunInput, file, message string) Finding {
	return Finding{Repo: in.Repo.Name, File: file, Message: message, Status: "open"}
}

// forbiddenPathMatches walks dir, returning every relative path that
// matches one of patterns (filepath.Match glob syntax against the
// whole relative path, doubled-star meaning "the rest of the path" —
// the same forbidden_paths shape the reference pack's own
// layout.json ships, e.g. "vendor/**").
func forbiddenPathMatches(dir string, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() && strings.HasPrefix(rel, ".git") {
			return filepath.SkipDir
		}
		for _, p := range patterns {
			if globMatch(p, rel) {
				out = append(out, rel)
				if d.IsDir() {
					return filepath.SkipDir
				}
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("check: layout: walk %s: %w", dir, err)
	}
	return out, nil
}

// globMatch matches pattern against path, where a trailing "/**"
// segment means "this prefix and everything under it" — filepath.Match
// alone treats "**" the same as "*" (no path-separator crossing), which
// would never match anything below the first directory level.
func globMatch(pattern, path string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	ok, _ := filepath.Match(pattern, path)
	return ok
}
