// Package detect is pure file-system probing: which languages, unit
// test runner, mutation tool and e2e suite a repository carries, and
// which changed files are "frontend" or "app" files. No function here
// runs a subprocess — every detector reads marker files (go.mod,
// package.json, ...) or matches paths, so this package needs no
// exec.Execer and no allowlist row. internal/verify's three
// subpackages consume Runners/E2E's results to decide what to run;
// internal/cli's `detect` command exposes each function directly.
package detect

import (
	"os"
	"path/filepath"
	"strings"
)

// Repo is the subset of repo.json's per-repository shape every
// detector needs: a name for the result it returns, and the local
// checkout to probe.
type Repo struct {
	Name      string
	LocalPath string
}

// hasFile reports whether dir/name exists as a regular file (or
// directory — callers that only ever name files get a file; this is
// intentionally not fussy about the distinction, since a marker's
// mere presence is what matters, not its type).
func hasFile(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// hasAnyGlob reports whether dir contains any entry whose name matches
// one of patterns (filepath.Match syntax, non-recursive) — used for
// marker files named by extension rather than by exact name (*.csproj,
// jest.config.*). One os.ReadDir per call, matched against every
// pattern, rather than one ReadDir per pattern.
func hasAnyGlob(dir string, patterns ...string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		for _, p := range patterns {
			if ok, _ := filepath.Match(p, e.Name()); ok {
				return true
			}
		}
	}
	return false
}

// filepathToSlash normalizes f to forward slashes — repo.json's
// in_scope_files are already git's own forward-slash paths, but this
// keeps path-segment matching correct if that ever changes.
func filepathToSlash(f string) string {
	return strings.ReplaceAll(f, "\\", "/")
}

// hasPathSegment reports whether f contains any of names as a whole
// path segment (not a substring match) — shared by Frontend's
// directory check and AppFiles' "docs/" exclusion.
func hasPathSegment(f string, names ...string) bool {
	for _, seg := range strings.Split(filepathToSlash(f), "/") {
		for _, want := range names {
			if seg == want {
				return true
			}
		}
	}
	return false
}

// readFile reads dir/name, returning (nil, false, nil) when it does
// not exist — every marker-file reader in this package treats a
// missing file as "no signal", not an error.
func readFile(dir, name string) ([]byte, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: dir is always a repo.json local_path, explicit CLI input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}
