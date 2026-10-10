package packs_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/packs"
)

// TestReferenceGitleaksScansMarkdown: secrets is the one check that
// cannot be waived, so the reference pack's own config must not exempt
// a consuming repository's Markdown (security review 2026-09-27 S5).
// It reads the embedded config's [allowlist] paths and asserts on what
// they match, not on how they are spelled.
func TestReferenceGitleaksScansMarkdown(t *testing.T) {
	data, err := packs.FS.ReadFile("reference/checks/gitleaks.toml")
	if err != nil {
		t.Fatal(err)
	}
	paths := allowlistPaths(t, string(data))
	if len(paths) == 0 {
		t.Fatal("no [allowlist] paths found; the parser below no longer matches the file")
	}
	for _, p := range paths {
		re, err := regexp.Compile(p)
		if err != nil {
			t.Fatalf("allowlist path %q does not compile: %v", p, err)
		}
		for _, file := range []string{"README.md", "docs/setup.md", "infra/notes.md"} {
			if re.MatchString(file) {
				t.Errorf("allowlist path %q exempts %s from the secrets scan", p, file)
			}
		}
	}
}

// allowlistPaths returns the triple-quoted literal entries of the paths array under
// [allowlist].
func allowlistPaths(t *testing.T, toml string) []string {
	t.Helper()
	_, section, ok := strings.Cut(toml, "[allowlist]")
	if !ok {
		t.Fatal("no [allowlist] table")
	}
	_, list, ok := strings.Cut(section, "paths = [")
	if !ok {
		t.Fatal("no paths array under [allowlist]")
	}
	list, _, _ = strings.Cut(list, "]")
	var out []string
	for _, m := range regexp.MustCompile(`'''(.*?)'''`).FindAllStringSubmatch(list, -1) {
		out = append(out, m[1])
	}
	return out
}
