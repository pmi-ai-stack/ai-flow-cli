package jenkins_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
)

func devConfig() *config.Config {
	return &config.Config{Ops: config.Ops{ToolTimeoutMinutes: 5}}
}

func boundConfig(env string) *config.Config {
	return &config.Config{Ops: config.Ops{
		ToolTimeoutMinutes: 5,
		Environments: map[string]config.Environment{
			env: {Tier: "nonprod", Jenkins: &config.JenkinsBinding{URL: "https://jenkins.example/" + env}},
		},
	}}
}

func TestNameAndCapabilities(t *testing.T) {
	a := jenkins.New(nil, devConfig())
	if a.Name() != "jenkins" {
		t.Errorf("Name() = %q, want jenkins", a.Name())
	}
	caps := a.Capabilities()
	if !caps.Render || !caps.Validate || !caps.Drift || !caps.DryRun || !caps.Rollback || !caps.UnitTest || !caps.E2ETest {
		t.Errorf("Capabilities() = %+v, want every column true", caps)
	}
}

func fixtureContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustCopyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
