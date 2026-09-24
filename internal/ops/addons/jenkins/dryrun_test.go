package jenkins_test

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func TestDryRunNoLiveJobIsCreate(t *testing.T) {
	branchDir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	x := exectest.New(t)
	target := ops.Target{Addon: "jenkins", Repo: "acme.svc", Path: "Jenkinsfile"}
	x.OnFailWithOutput("jenkins-cli", []string{"get-job", "acme.svc", "-s", "https://jenkins.example/dev"},
		exec.Output{Stderr: "No such job 'acme.svc'", ExitCode: 3},
		&envelope.Error{Code: "tool_failed", Message: "no such job"},
	)

	a := jenkins.New(x, boundConfig("dev"))
	cs, err := a.DryRun(context.Background(), target, ops.Env{Name: "dev", Tier: "nonprod"}, &ops.Rendered{Dir: branchDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 1 || cs.Actions[0].Action != ops.ActionCreate {
		t.Fatalf("Actions = %v, want a single create", cs.Actions)
	}
}

func TestDryRunNoDeltaIsEmptyChangeset(t *testing.T) {
	branchDir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	x := exectest.New(t)
	target := ops.Target{Addon: "jenkins", Repo: "acme.svc", Path: "Jenkinsfile"}
	x.On("jenkins-cli", []string{"get-job", "acme.svc", "-s", "https://jenkins.example/dev"}, exec.Output{Stdout: fixtureContent(t, "testdata/Jenkinsfile")})

	a := jenkins.New(x, boundConfig("dev"))
	cs, err := a.DryRun(context.Background(), target, ops.Env{Name: "dev", Tier: "nonprod"}, &ops.Rendered{Dir: branchDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 0 {
		t.Errorf("Actions = %v, want none (live already matches branch)", cs.Actions)
	}
}

func TestDryRunUpdateOnRealDelta(t *testing.T) {
	branchDir := renderedDir(t, "Jenkinsfile", "testdata/Jenkinsfile")
	x := exectest.New(t)
	target := ops.Target{Addon: "jenkins", Repo: "acme.svc", Path: "Jenkinsfile"}
	x.On("jenkins-cli", []string{"get-job", "acme.svc", "-s", "https://jenkins.example/dev"}, exec.Output{Stdout: "pipeline { agent any }"})

	a := jenkins.New(x, boundConfig("dev"))
	cs, err := a.DryRun(context.Background(), target, ops.Env{Name: "dev", Tier: "nonprod"}, &ops.Rendered{Dir: branchDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 1 || cs.Actions[0].Action != ops.ActionUpdate || cs.Actions[0].KindClass != ops.KindPipeline {
		t.Fatalf("Actions = %v, want a single update, kind_class pipeline", cs.Actions)
	}
}
