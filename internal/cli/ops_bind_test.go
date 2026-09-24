package cli

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
)

// TestRunOpsDetectGithubActionsAlwaysUnbound proves the phase-27
// correction to runOpsDetect's own binding loop: a github-actions
// target binds the unbound environment even when config.ops.target_map
// names one explicitly for its exact path — the spec's own "a
// workflow file... has no live counterpart at all" rule, applied
// unconditionally.
func TestRunOpsDetectGithubActionsAlwaysUnbound(t *testing.T) {
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, ".github", "workflows", "ci.yml"), "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")

	cfg := &config.Config{Ops: config.Ops{
		TargetMap:    map[string]string{"acme.svc:.github/workflows/ci.yml": "dev"},
		Environments: map[string]config.Environment{"dev": {Tier: "nonprod"}},
	}}
	artifact := &repoArtifact{Repositories: []repoArtifactRepository{{Name: "acme.svc", LocalPath: repoDir}}}

	result, err := runOpsDetect(Flags{}, cfg, artifact, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 {
		t.Fatalf("Targets = %v, want exactly one workflow target", result.Targets)
	}
	got := result.Targets[0]
	if got.Environment != nil {
		t.Errorf("Environment = %v, want nil (unbound) even though target_map names dev", *got.Environment)
	}
	if got.Tier == nil || *got.Tier != "nonprod" {
		t.Errorf("Tier = %v, want nonprod (the unbound environment's own tier)", got.Tier)
	}
}

// TestRunOpsDetectJenkinsReboundWhenEnvHasNoJenkinsBinding proves the
// second half of the same correction: a jenkins target resolved to an
// environment with no jenkins binding configured is rebound to
// unbound, not left pointing at an environment with nothing to bind
// to (the spec's own "a Jenkins job DSL with no configured URL" case).
func TestRunOpsDetectJenkinsReboundWhenEnvHasNoJenkinsBinding(t *testing.T) {
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "Jenkinsfile"), "pipeline { agent any }\n")

	cfg := &config.Config{Ops: config.Ops{
		DefaultEnvironment: "dev",
		Environments:       map[string]config.Environment{"dev": {Tier: "nonprod"}}, // no Jenkins binding
	}}
	artifact := &repoArtifact{Repositories: []repoArtifactRepository{{Name: "acme.svc", LocalPath: repoDir}}}

	result, err := runOpsDetect(Flags{}, cfg, artifact, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 {
		t.Fatalf("Targets = %v, want exactly one Jenkinsfile target", result.Targets)
	}
	got := result.Targets[0]
	if got.Environment != nil {
		t.Errorf("Environment = %v, want nil (rebound to unbound — dev has no jenkins binding)", *got.Environment)
	}
}

// TestRunOpsDetectJenkinsBindsWhenEnvHasJenkinsBinding is the positive
// case sitting beside the above: a jenkins target resolved to an
// environment that DOES carry a jenkins binding stays bound to it.
func TestRunOpsDetectJenkinsBindsWhenEnvHasJenkinsBinding(t *testing.T) {
	repoDir := t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "Jenkinsfile"), "pipeline { agent any }\n")

	cfg := &config.Config{Ops: config.Ops{
		DefaultEnvironment: "dev",
		Environments: map[string]config.Environment{
			"dev": {Tier: "nonprod", Jenkins: &config.JenkinsBinding{URL: "https://jenkins.example/dev"}},
		},
	}}
	artifact := &repoArtifact{Repositories: []repoArtifactRepository{{Name: "acme.svc", LocalPath: repoDir}}}

	result, err := runOpsDetect(Flags{}, cfg, artifact, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 {
		t.Fatalf("Targets = %v, want exactly one Jenkinsfile target", result.Targets)
	}
	got := result.Targets[0]
	if got.Environment == nil || *got.Environment != "dev" {
		t.Errorf("Environment = %v, want dev (a real jenkins binding exists)", got.Environment)
	}
}
