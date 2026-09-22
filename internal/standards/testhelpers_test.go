package standards

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// writeFile writes content to path, creating parent directories as
// needed — shared by every test in this package that builds an ad hoc
// fixture under t.TempDir() rather than reading one from testdata/.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// configWithSources builds a *config.Config carrying one org source and
// two team sources under team names that sort the opposite way from Go
// map iteration would give by chance — SourcesFromConfig must still
// return them alpha before beta regardless.
func configWithSources() *config.Config {
	return &config.Config{
		Standards: config.Standards{
			Sources: []config.StandardsSource{{ID: "acme-org", Repo: "https://example.com/org.git", Ref: "v1.0.0"}},
		},
		Routing: config.Routing{
			Teams: map[string]config.Team{
				"Zulu Team":  {Standards: []config.StandardsSource{{ID: "beta-team-pack", Repo: "https://example.com/zulu.git", Ref: "main"}}},
				"Alpha Team": {Standards: []config.StandardsSource{{ID: "alpha-team-pack", Repo: "https://example.com/alpha.git", Ref: "main"}}},
			},
		},
	}
}
