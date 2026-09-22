package mutation

import (
	"path"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// mutable reports whether file — a repo-relative path from a diff —
// is a source file the named tool would mutate: the right extension
// for its language, and not itself a test file by that language's
// dominant naming convention. The plan's design decision 12.
func mutable(tool, file string) bool {
	base := path.Base(file)
	switch tool {
	case "gremlins":
		return strings.HasSuffix(file, ".go") && !strings.HasSuffix(file, "_test.go")
	case "stryker":
		if !hasAnySuffix(file, ".js", ".jsx", ".ts", ".tsx") {
			return false
		}
		if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
			return false
		}
		return !strutil.HasPathSegment(file, "__tests__")
	case "mutmut":
		if !strings.HasSuffix(file, ".py") {
			return false
		}
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
			return false
		}
		return !strutil.HasPathSegment(file, "tests")
	case "pitest":
		if !strings.HasSuffix(file, ".java") {
			return false
		}
		if strings.HasSuffix(base, "Test.java") || strings.HasPrefix(base, "Test") {
			return false
		}
		return !strutil.HasPathSegment(file, "test")
	default:
		return false
	}
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

// ScopeFor narrows diffFiles to repo's changed-code scope for tool —
// the pure function both `mutation scope` (read-only) and `mutation
// run` (immediately before executing the tool) call, so the two never
// disagree (design decision 3). diffFiles is repo-relative, exactly
// gitops.Diff's own Files shape.
func ScopeFor(repo detect.Repo, tool string, diffFiles []string) Scope {
	var files []string
	for _, f := range diffFiles {
		if mutable(tool, f) {
			files = append(files, f)
		}
	}
	return Scope{Repo: repo.Name, Files: files}
}
