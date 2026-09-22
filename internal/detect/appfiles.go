package detect

import (
	"path"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// documentationExtensions excludes a changed file from "app files" by
// its extension.
var documentationExtensions = []string{".md", ".rst", ".txt"}

// documentationBasenamePrefixes excludes a changed file from "app
// files" by its base name — case-sensitive, matching the conventional
// all-caps spelling (LICENSE, CHANGELOG.md already caught by the
// extension rule too, LICENSE.txt likewise).
var documentationBasenamePrefixes = []string{"LICENSE", "CHANGELOG"}

func isDocumentation(f string) bool {
	base := path.Base(f)
	ext := strings.ToLower(path.Ext(f))
	for _, want := range documentationExtensions {
		if ext == want {
			return true
		}
	}
	for _, prefix := range documentationBasenamePrefixes {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return hasPathSegment(f, "docs")
}

// AppFiles is "changed files no addon claimed and that are not
// documentation" (Workflow's own definition). claimedByAddon is empty
// on every call through phase 17 — no OPS package exists yet to claim
// anything — the parameter exists so phase 18 adds one argument at its
// own call site rather than changing this function's shape.
func AppFiles(changed []string, claimedByAddon []string) []string {
	claimed := make(map[string]bool, len(claimedByAddon))
	for _, f := range claimedByAddon {
		claimed[f] = true
	}

	var app []string
	for _, f := range changed {
		if isDocumentation(f) || claimed[f] {
			continue
		}
		app = append(app, f)
	}

	result := strutil.DedupSorted(app)
	if result == nil {
		result = []string{}
	}
	return result
}
