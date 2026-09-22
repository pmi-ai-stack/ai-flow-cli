package detect

import (
	"path"
	"strings"
)

// frontendExtensions is checked against each changed file's extension.
var frontendExtensions = []string{".tsx", ".jsx", ".vue", ".svelte", ".css", ".scss", ".html"}

// frontendPathSegments is checked against each changed file's path
// segments — a file inside one of these directories is frontend code
// even when its own extension is language-neutral (.ts, .js).
var frontendPathSegments = []string{"components", "pages", "views", "styles"}

// Frontend reports whether files (repo.json's in_scope_files — already
// computed by `repo scope`) contains any recognizable frontend file.
// Pure path matching, no filesystem access: the diff already tells us
// everything this needs.
func Frontend(files []string) bool {
	for _, f := range files {
		ext := strings.ToLower(path.Ext(f))
		for _, want := range frontendExtensions {
			if ext == want {
				return true
			}
		}
		if hasPathSegment(f, frontendPathSegments...) {
			return true
		}
	}
	return false
}
