package jenkins_test

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
)

func TestHarnessesDetectsJenkinsPipelineUnitAndAlwaysE2E(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/Jenkinsfile", filepath.Join(repoDir, "Jenkinsfile"))
	mustCopyFile(t, "testdata/BuildSpec.groovy", filepath.Join(repoDir, "test", "BuildSpec.groovy"))
	mustCopyFile(t, "testdata/jobs/build.groovy", filepath.Join(repoDir, "jobs", "build.groovy")) // not a *Spec, no import — must not be claimed as unit

	a := jenkins.New(nil, devConfig())
	target := ops.Target{Addon: "jenkins", Repo: "acme.svc", Path: "Jenkinsfile"}
	harnesses, err := a.Harnesses(detect.Repo{Name: "acme.svc", LocalPath: repoDir}, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(harnesses) != 2 {
		t.Fatalf("harnesses = %v, want exactly 2 (one unit, one e2e)", harnesses)
	}
	var unit, e2e int
	for _, h := range harnesses {
		switch h.Kind {
		case ops.HarnessUnit:
			unit++
		case ops.HarnessE2E:
			e2e++
			if h.Name != "Jenkinsfile#jenkinsfile-runner" {
				t.Errorf("e2e harness Name = %q, want Jenkinsfile#jenkinsfile-runner", h.Name)
			}
		}
	}
	if unit != 1 || e2e != 1 {
		t.Errorf("unit=%d e2e=%d, want 1 and 1", unit, e2e)
	}
}
