package detect

// E2EMatch is the detected e2e suite for one repository. Only
// Playwright is detected today (the plan's design decision 3) — a
// second framework is a second branch in E2E plus a second parser in
// internal/verify/e2e, not a signature change. No Repo field — see
// RunnerMatch's own doc comment.
type E2EMatch struct {
	Suite   string // "playwright"
	Command string
	Args    []string
}

// E2E detects repo's e2e suite. ok is false when nothing matches.
func E2E(repo Repo) (E2EMatch, bool, error) {
	dir := repo.LocalPath

	if hasAnyGlob(dir, "playwright.config.js", "playwright.config.ts", "playwright.config.mjs", "playwright.config.cjs") {
		return E2EMatch{
			Suite:   "playwright",
			Command: "npx",
			Args:    []string{"playwright", "test", "--reporter=json"},
		}, true, nil
	}

	return E2EMatch{}, false, nil
}
