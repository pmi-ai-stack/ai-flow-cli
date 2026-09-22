package detect

import "github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"

// Languages returns repo's detected languages, by marker file — the
// five the reference pack (phase 12) carries guidance for. Sorted,
// deduped; nil is never returned in place of an empty slice's zero
// value (strutil.DedupSorted already normalizes that).
func Languages(repo Repo) ([]string, error) {
	dir := repo.LocalPath
	var langs []string

	if hasFile(dir, "go.mod") {
		langs = append(langs, "go")
	}
	if hasFile(dir, "package.json") {
		langs = append(langs, "javascript")
		if hasFile(dir, "tsconfig.json") {
			langs = append(langs, "typescript")
		}
	}
	if hasFile(dir, "pyproject.toml") || hasFile(dir, "setup.py") || hasFile(dir, "requirements.txt") {
		langs = append(langs, "python")
	}
	if hasAnyGlob(dir, "*.csproj") {
		langs = append(langs, "csharp")
	}
	if hasFile(dir, "pom.xml") || hasFile(dir, "build.gradle") || hasFile(dir, "build.gradle.kts") {
		langs = append(langs, "java")
	}

	result := strutil.DedupSorted(langs)
	if result == nil {
		result = []string{}
	}
	return result, nil
}
