package jenkins_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func renderedDir(t *testing.T, name, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	mustCopyFile(t, fixture, filepath.Join(dir, name))
	return dir
}

func TestValidateSkipsNonJenkinsfileTargets(t *testing.T) {
	x := exectest.New(t)
	dir := renderedDir(t, "build.xml", "testdata/jobs/build.xml")
	a := jenkins.New(x, boundConfig("dev"))

	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 || len(x.Calls) != 0 {
		t.Errorf("findings=%v calls=%v, want no findings and no subprocess for a non-Jenkinsfile target", findings, x.Calls)
	}
}

func TestValidateNoBindingIsSoftFinding(t *testing.T) {
	x := exectest.New(t)
	dir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	a := jenkins.New(x, devConfig()) // no environments configured at all

	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Severity != "should" {
		t.Fatalf("findings = %v, want one should-severity soft finding", findings)
	}
	if len(x.Calls) != 0 {
		t.Errorf("Calls = %v, want none — no binding means no subprocess", x.Calls)
	}
}

func TestValidateCallsDeclarativeLinterWithStdin(t *testing.T) {
	x := exectest.New(t)
	x.On("jenkins-cli", []string{"declarative-linter", "-s", "https://jenkins.example/dev"}, exec.Output{})
	dir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	a := jenkins.New(x, boundConfig("dev"))

	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none on a clean lint", findings)
	}
	if len(x.Calls) != 1 || x.Calls[0].Stdin == "" {
		t.Fatalf("Calls = %v, want exactly one call carrying the Jenkinsfile content on stdin", x.Calls)
	}
}

func TestValidateReportsDeclarativeLinterFailure(t *testing.T) {
	x := exectest.New(t)
	x.OnFailWithOutput("jenkins-cli", []string{"declarative-linter", "-s", "https://jenkins.example/dev"},
		exec.Output{Stderr: "WorkflowScript: 3: Invalid agent", ExitCode: 1},
		&envelope.Error{Code: "tool_failed", Message: "jenkins-cli: invalid pipeline"},
	)
	dir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	a := jenkins.New(x, boundConfig("dev"))

	findings, err := a.Validate(context.Background(), &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Severity != "error" {
		t.Fatalf("findings = %v, want one error-severity finding", findings)
	}
}
