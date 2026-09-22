package visual

import (
	"path"
	"strings"
)

// Slug turns a screen path ("/", "/dashboard/settings") into a
// filesystem-safe name for its screenshot file — shared by `visual
// run` (which writes screenshots/<slug>.png) and `visual judge` (which
// looks one up by the same rule).
func Slug(screen string) string {
	s := strings.Trim(screen, "/")
	s = strings.ReplaceAll(s, "/", "_")
	if s == "" {
		return "home"
	}
	return s
}

// screenPathSegments is checked against each candidate file's path
// segments to recognize it as naming a screen (a routed page), not
// merely a frontend file in general (frontend.go's broader net).
var screenPathSegments = []string{"pages", "routes", "views"}

// DefaultScreens derives a best-effort screen list from changed files,
// for `visual run` when the skill names none with --screen: a file
// under a pages/routes/views directory becomes a URL path from its own
// repo-relative path (extension stripped, a trailing "/index"
// collapsed to "/"); no recognizable file at all falls back to ["/"].
// This is a heuristic, not a contract — --screen always overrides it,
// and the skill is expected to name additional screens the diff
// implies but this function's path-guessing cannot infer.
func DefaultScreens(files []string) []string {
	var screens []string
	seen := map[string]bool{}
	for _, f := range files {
		if p, ok := screenFromPath(f); ok && !seen[p] {
			seen[p] = true
			screens = append(screens, p)
		}
	}
	if len(screens) == 0 {
		return []string{"/"}
	}
	return screens
}

// screenFromPath extracts a URL path from f when f sits under one of
// screenPathSegments.
func screenFromPath(f string) (string, bool) {
	segs := strings.Split(strings.ReplaceAll(f, "\\", "/"), "/")
	for i, seg := range segs {
		for _, want := range screenPathSegments {
			if seg != want {
				continue
			}
			rest := segs[i+1:]
			if len(rest) == 0 {
				return "", false
			}
			last := len(rest) - 1
			rest[last] = strings.TrimSuffix(rest[last], path.Ext(rest[last]))
			if rest[last] == "index" {
				rest = rest[:last]
			}
			p := "/" + strings.Join(rest, "/")
			if p == "/" {
				return "/", true
			}
			return p, true
		}
	}
	return "", false
}
