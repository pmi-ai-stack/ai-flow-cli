package jenkins_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// The Jenkins credentials reach jenkins-cli through its own
// JENKINS_USER_ID/JENKINS_API_TOKEN environment variables, never argv,
// where any local user can read them from ps or /proc (2026-10-04
// security review #4). The scripted argv below carries no -auth, so a
// call that still passed one would be an unexpected call and fail.

func requireEnvCredentials(t *testing.T, calls []exectest.Call) {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("Calls = %v, want exactly one", calls)
	}
	c := calls[0]
	for _, a := range c.Args {
		if strings.Contains(a, "s3cret") {
			t.Errorf("argv carries the token: %v", c.Args)
		}
	}
	for _, want := range []string{"JENKINS_USER_ID=bot", "JENKINS_API_TOKEN=s3cret"} {
		if !slices.Contains(c.Env, want) {
			t.Errorf("Env = %v, want %s", c.Env, want)
		}
	}
}

func TestValidatePassesCredentialsInEnvNotArgv(t *testing.T) {
	t.Setenv("AI_FLOW_JENKINS_USER", "bot")
	t.Setenv("AI_FLOW_JENKINS_TOKEN", "s3cret")
	x := exectest.New(t)
	x.On("jenkins-cli", []string{"declarative-linter", "-s", "https://jenkins.example/dev"}, exec.Output{})
	dir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")

	if _, err := jenkins.New(x, boundConfig("dev")).Validate(context.Background(), &ops.Rendered{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	requireEnvCredentials(t, x.Calls)
}

func TestDriftPassesCredentialsInEnvNotArgv(t *testing.T) {
	t.Setenv("AI_FLOW_JENKINS_USER", "bot")
	t.Setenv("AI_FLOW_JENKINS_TOKEN", "s3cret")
	baseDir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	x := exectest.New(t)
	target := ops.Target{Addon: "jenkins", Repo: "acme.svc", Path: "Jenkinsfile"}
	x.On("jenkins-cli", []string{"get-job", "acme.svc", "-s", "https://jenkins.example/dev"}, exec.Output{Stdout: fixtureContent(t, "testdata/Jenkinsfile")})

	if _, err := jenkins.New(x, boundConfig("dev")).Drift(context.Background(), target, ops.Env{Name: "dev"}, &ops.Rendered{Dir: baseDir}); err != nil {
		t.Fatal(err)
	}
	requireEnvCredentials(t, x.Calls)
}

func TestAnonymousJenkinsAddsNoEnv(t *testing.T) {
	t.Setenv("AI_FLOW_JENKINS_USER", "")
	t.Setenv("AI_FLOW_JENKINS_TOKEN", "")
	x := exectest.New(t)
	x.On("jenkins-cli", []string{"declarative-linter", "-s", "https://jenkins.example/dev"}, exec.Output{})
	dir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")

	if _, err := jenkins.New(x, boundConfig("dev")).Validate(context.Background(), &ops.Rendered{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if len(x.Calls) != 1 || len(x.Calls[0].Env) != 0 {
		t.Errorf("Calls = %+v, want one call with no extra environment", x.Calls)
	}
}
