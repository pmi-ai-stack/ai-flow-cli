package terraform

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// terraformBlock matches a top-level "terraform {" block opener —
// Detect's own root-module signal (spec: "root module (terraform {}
// or backend)"; a bare backend block is itself always nested inside a
// terraform {} block in valid HCL, so this one regex covers both
// phrasings). A plain text scan, not a full HCL parse: no HCL library
// is a dependency of this module (the phase-21 plan's own §0.9), and
// this heuristic needs nothing more.
var terraformBlock = regexp.MustCompile(`(?m)^\s*terraform\s*\{`)

// skipDir reports whether name is never descended into while walking
// for root modules or copying one: .terraform is a prior init's own
// provider/module cache (Render always starts from a clean copy, so a
// checked-out .terraform/ must never be mistaken for module source or
// carried into a new rendering), and .git or any other dot-directory
// is conventionally not terraform content either.
func skipDir(name string) bool {
	return name == ".terraform" || strings.HasPrefix(name, ".")
}

// Detect walks repo's checkout for every directory whose own .tf
// files contain a top-level "terraform {}" block — one ops.Target per
// directory found, Path "/"-separated relative to repo.LocalPath.
// changed is nil at scope "repo" (Target.Changed's own doc comment);
// a non-nil changed is matched by path-or-prefix exactly like
// fixtureaddon.Detect already does, reused rather than reinvented.
//
// found is built directly from the walk's own per-file callbacks
// (grouped by each file's parent directory) rather than a second,
// explicit os.ReadDir per directory: filepath.WalkDir already lists
// every directory's entries once, internally, to recurse into them and
// to invoke this callback for each one — a second listing would be a
// redundant syscall per directory for no new information. A directory
// already found true stops reading its own later .tf files, the same
// short-circuit the previous per-directory os.ReadDir loop had.
func (a *Addon) Detect(repo detect.Repo, changed []string) ([]ops.Target, error) {
	found := map[string]bool{}
	err := filepath.WalkDir(repo.LocalPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != repo.LocalPath && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		dir := filepath.Dir(path)
		if found[dir] || !strings.HasSuffix(d.Name(), ".tf") {
			return nil
		}
		ok, err := containsTerraformBlock(path)
		if err != nil {
			return err
		}
		if ok {
			found[dir] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("terraform: detect: %w", err)
	}

	dirs := make([]string, 0, len(found))
	for dir := range found {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	targets := make([]ops.Target, 0, len(dirs))
	for _, dir := range dirs {
		rel, err := filepath.Rel(repo.LocalPath, dir)
		if err != nil {
			return nil, fmt.Errorf("terraform: detect: %w", err)
		}
		p := filepath.ToSlash(rel)
		targets = append(targets, ops.Target{Addon: "terraform", Path: p, Files: filesUnder(p, changed)})
	}
	return targets, nil
}

// containsTerraformBlock reports whether the .tf file at path contains
// a top-level terraform {} block.
func containsTerraformBlock(path string) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from WalkDir over repo.LocalPath, a checkout ai-flow itself resolved
	if err != nil {
		return false, fmt.Errorf("terraform: read %s: %w", path, err)
	}
	return terraformBlock.Match(data), nil
}

// filesUnder mirrors fixtureaddon.Detect's own changed-file matching:
// every entry of changed that equals path or lies under path+"/".
func filesUnder(path string, changed []string) []string {
	var files []string
	prefix := path + "/"
	for _, f := range changed {
		if f == path || strings.HasPrefix(f, prefix) {
			files = append(files, f)
		}
	}
	return files
}

// SkipTreeDir implements ops.TreeSkipper with the same skipDir rule
// Render's copyTree applies, so render.TreeHash never hashes content
// the rendering cannot include.
func (a *Addon) SkipTreeDir(name string) bool {
	return skipDir(name)
}
