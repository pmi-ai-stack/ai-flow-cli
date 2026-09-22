package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fixtureRepoPath resolves one of internal/detect's shared fixture
// repositories (testdata/repos/{go,typescript,python}, built for
// phase 6 — see internal/detect's own tests) to an absolute path, so
// this package's tests can point a repo.json fixture's local_path at a
// real, marker-file-carrying checkout without duplicating those
// fixtures here.
func fixtureRepoPath(t *testing.T, lang string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "repos", lang))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return abs
}

// fixtureRepo is one (name, local path) pair for writeRepoJSONFixture —
// a slice, not a map, because repo.json's repositories[] is an
// ordered array and several phase-6 commands (findE2ERepo,
// findVisualRepo) deliberately pick "the first repository that
// matches", which a map's randomized iteration order would make these
// tests flaky against.
type fixtureRepo struct {
	name, path string
}

// writeRepoJSONFixture writes a repo.json for key under home, with one
// repository per entry in repos, in order — the shape every phase-6
// CLI command under test here reads via loadRepoArtifact.
func writeRepoJSONFixture(t *testing.T, home, key string, repos []fixtureRepo, inScopeFiles []string) {
	t.Helper()
	artifact := repoArtifact{Team: "Team Beta", InScopeFiles: inScopeFiles}
	for _, r := range repos {
		artifact.Repositories = append(artifact.Repositories, repoArtifactRepository{Name: r.name, LocalPath: r.path, InstructionsFile: "AGENTS.md"})
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("marshal repo.json: %v", err)
	}
	dir := filepath.Join(home, key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "repo.json"), data, 0o600); err != nil {
		t.Fatalf("write repo.json: %v", err)
	}
}
