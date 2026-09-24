package githubactions_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

func renderedDir(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	mustCopyFile(t, fixture, filepath.Join(dir, githubactions.RenderedFile))
	return dir
}

func TestDryRunReportsJobIDsAsOneUpdateAction(t *testing.T) {
	dir := renderedDir(t, "testdata/ci-two-jobs.yml")
	x := exectest.New(t)
	x.On("act", []string{"-n", "-W", githubactions.RenderedFile}, exec.Output{})

	a := githubactions.New(x, devConfig())
	target := ops.Target{Addon: "github-actions", Repo: "acme.svc", Path: ".github/workflows/ci.yml"}
	cs, err := a.DryRun(context.Background(), target, ops.Env{}, &ops.Rendered{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Actions) != 1 {
		t.Fatalf("Actions = %v, want exactly one", cs.Actions)
	}
	act := cs.Actions[0]
	if act.Action != ops.ActionUpdate || act.Address != target.Path || act.KindClass != ops.KindPipeline {
		t.Errorf("action = %+v, unexpected", act)
	}
	if len(act.FieldsChanged) != 2 || act.FieldsChanged[0] != "build" || act.FieldsChanged[1] != "test" {
		t.Errorf("FieldsChanged = %v, want [build test]", act.FieldsChanged)
	}
}

func TestDryRunPropagatesActFailure(t *testing.T) {
	dir := renderedDir(t, "testdata/broken.yml")
	x := exectest.New(t)
	x.OnFail("act", []string{"-n", "-W", githubactions.RenderedFile}, &envelope.Error{Code: "tool_failed", Message: "workflow is not valid"})

	a := githubactions.New(x, devConfig())
	target := ops.Target{Addon: "github-actions", Repo: "acme.svc", Path: ".github/workflows/broken.yml"}
	if _, err := a.DryRun(context.Background(), target, ops.Env{}, &ops.Rendered{Dir: dir}); err == nil {
		t.Fatal("DryRun() err = nil, want the act failure propagated")
	}
}
