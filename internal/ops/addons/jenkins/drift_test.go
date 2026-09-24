package jenkins_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestDriftUnboundEnvIsLiveCredentialsMissingWithNoCalls(t *testing.T) {
	x := exectest.New(t)
	a := jenkins.New(x, devConfig())
	_, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: ""}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("err = %v, want ops.ErrLiveCredentialsMissing", err)
	}
	if len(x.Calls) != 0 {
		t.Errorf("Calls = %v, want none", x.Calls)
	}
}

func TestDriftUnconfiguredEnvIsLiveCredentialsMissing(t *testing.T) {
	x := exectest.New(t)
	cfg := devConfig()
	a := jenkins.New(x, cfg)
	_, err := a.Drift(context.Background(), ops.Target{}, ops.Env{Name: "staging"}, &ops.Rendered{Dir: t.TempDir()})
	if !errors.Is(err, ops.ErrLiveCredentialsMissing) {
		t.Fatalf("err = %v, want ops.ErrLiveCredentialsMissing (staging has no jenkins binding)", err)
	}
}

func TestDriftNoneWhenLiveMatchesBase(t *testing.T) {
	baseDir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	x := exectest.New(t)
	target := ops.Target{Addon: "jenkins", Repo: "acme.svc", Path: "Jenkinsfile"}
	x.On("jenkins-cli", []string{"get-job", "acme.svc", "-s", "https://jenkins.example/dev"}, exec.Output{Stdout: fixtureContent(t, "testdata/Jenkinsfile")})

	a := jenkins.New(x, boundConfig("dev"))
	report, err := a.Drift(context.Background(), target, ops.Env{Name: "dev"}, &ops.Rendered{Dir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftNone {
		t.Errorf("Drift = %q, want none", report.Drift)
	}
}

func TestDriftPresentWhenLiveDiffersFromBase(t *testing.T) {
	baseDir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	x := exectest.New(t)
	target := ops.Target{Addon: "jenkins", Repo: "acme.svc", Path: "Jenkinsfile"}
	x.On("jenkins-cli", []string{"get-job", "acme.svc", "-s", "https://jenkins.example/dev"}, exec.Output{Stdout: "pipeline { agent any }"})

	a := jenkins.New(x, boundConfig("dev"))
	report, err := a.Drift(context.Background(), target, ops.Env{Name: "dev"}, &ops.Rendered{Dir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftPresent || len(report.Resources) != 1 || report.Resources[0] != "acme.svc" {
		t.Errorf("report = %+v, want present naming acme.svc", report)
	}
}
