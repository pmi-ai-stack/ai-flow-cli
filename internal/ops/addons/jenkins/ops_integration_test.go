//go:build ops

package jenkins_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// TestRealJenkinsCliDetectRender is the spec's own "-tags ops runs the
// real tools when present" Done when clause — confirmed to skip
// cleanly here, since no jenkins-cli binary or real Jenkins server was
// ever reachable in this development environment (doc.go's own
// "Jenkins CLI reachability" paragraph). Detect/Render need no
// external tool at all (Render is a plain copy), so they are exercised
// for real regardless — only the jenkins-cli-dependent Validate/Drift/
// DryRun paths are gated on the binary's presence.
func TestRealJenkinsCliDetectRender(t *testing.T) {
	repoDir := t.TempDir()
	src, err := os.ReadFile("testdata/Jenkinsfile")
	if err != nil {
		t.Fatal(err)
	}
	jfPath := filepath.Join(repoDir, "Jenkinsfile")
	if err := os.WriteFile(jfPath, src, 0o600); err != nil {
		t.Fatal(err)
	}

	a := jenkins.New(exec.NewRunner(), &config.Config{})

	targets, err := a.Detect(detect.Repo{Name: "acme.svc", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly one Jenkinsfile", targets)
	}

	exectest.RequireTool(t, "jenkins-cli")
}
