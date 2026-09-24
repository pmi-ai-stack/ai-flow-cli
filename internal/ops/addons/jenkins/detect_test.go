package jenkins_test

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
)

func TestDetectFindsAllThreeSignals(t *testing.T) {
	repoDir := t.TempDir()
	mustCopyFile(t, "testdata/Jenkinsfile", filepath.Join(repoDir, "Jenkinsfile"))
	mustCopyFile(t, "testdata/jobs/build.xml", filepath.Join(repoDir, "jobs", "build.xml"))
	mustCopyFile(t, "testdata/jobs/build.groovy", filepath.Join(repoDir, "jobs", "build.groovy"))
	// An unrelated top-level .groovy file must NOT be claimed as a
	// jenkins target — job DSL detection is scoped to jobs/ (design
	// decision 6).
	mustCopyFile(t, "testdata/jobs/build.groovy", filepath.Join(repoDir, "buildSrc", "helpers.groovy"))

	a := jenkins.New(nil, devConfig())
	targets, err := a.Detect(detect.Repo{Name: "acme.svc", LocalPath: repoDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Fatalf("targets = %v, want exactly 3", targets)
	}
	paths := map[string]bool{}
	for _, tg := range targets {
		paths[tg.Path] = true
		if tg.Addon != "jenkins" {
			t.Errorf("target %+v has Addon != jenkins", tg)
		}
	}
	for _, want := range []string{"Jenkinsfile", "jobs/build.xml", "jobs/build.groovy"} {
		if !paths[want] {
			t.Errorf("targets = %v, missing %q", targets, want)
		}
	}
}

func TestDetectEmptyRepoIsEmptyNotError(t *testing.T) {
	a := jenkins.New(nil, devConfig())
	targets, err := a.Detect(detect.Repo{Name: "acme.svc", LocalPath: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Errorf("targets = %v, want none", targets)
	}
}
