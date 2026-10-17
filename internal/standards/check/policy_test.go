package check_test

import (
	"context"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

func TestRunPolicyConftestSourceInputFindsFailure(t *testing.T) {
	report := `[{"filename":"main.tf","namespace":"main","failures":[{"msg":"missing tags"}],"warnings":[{"msg":"consider a description"}]}]`
	x := exectest.New(t)
	// The policy dir is materialized to a deterministic temp path this
	// test does not know ahead of time, so it registers a response for
	// every conftest call this test could make by matching on argv
	// shape via a wrapping Execer instead of a literal path.
	fake := &conftestExecer{Scripted: x, out: exec.Output{Stdout: report}}

	c := standards.EffectiveCheck{
		ID: "infra.terraform.policy", Kind: "policy", Tool: "conftest", Severity: "must",
		Config: "policy", Pack: packWithConfigDir("policy", map[string]string{"naming.rego": "package main\n"}),
	}
	runner, ok := check.ForKind("policy")
	if !ok {
		t.Fatal("no policy runner registered")
	}
	findings, err := runner.Run(context.Background(), check.RunInput{
		Scratch: t.TempDir(),
		Check:   c,
		Target:  check.Target{Key: "acme.infra:modules/vpc", Addon: "terraform", Repo: "acme.infra", SourceDir: "/repo/modules/vpc"},
	}, fake, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v, want 2", findings)
	}
	failure, warning := findings[0], findings[1]
	if failure.Message != "missing tags" || failure.Severity != "" || failure.Status != "open" {
		t.Errorf("failure finding = %+v", failure)
	}
	wantWarnSeverity := check.CapSeverity("medium", c.Severity)
	if warning.Message != "consider a description" || warning.Severity != wantWarnSeverity {
		t.Errorf("warning finding = %+v, want severity %q", warning, wantWarnSeverity)
	}
	if failure.Target != "acme.infra:modules/vpc" || failure.Repo != "acme.infra" {
		t.Errorf("finding target/repo = %q/%q", failure.Target, failure.Repo)
	}
	if len(fake.calls) != 1 || fake.calls[0].dir != "/repo/modules/vpc" {
		t.Errorf("conftest call = %+v, want one call with dir /repo/modules/vpc", fake.calls)
	}
}

func TestRunPolicyConftestRenderInputCleanRun(t *testing.T) {
	x := exectest.New(t)
	fake := &conftestExecer{Scripted: x, out: exec.Output{Stdout: `[{"filename":"deploy.yaml","namespace":"main","failures":[],"warnings":[]}]`}}

	c := standards.EffectiveCheck{
		ID: "infra.kubernetes.policy", Kind: "policy", Tool: "conftest", Severity: "must", Input: "render",
		Config: "policy", Pack: packWithConfigDir("policy", map[string]string{"labels.rego": "package main\n"}),
	}
	runner, _ := check.ForKind("policy")
	findings, err := runner.Run(context.Background(), check.RunInput{
		Scratch: t.TempDir(),
		Check:   c,
		Target: check.Target{
			Key: "acme.k8s:overlays/prod", Addon: "kubectl", Repo: "acme.k8s",
			SourceDir: "/repo/overlays/prod", RenderDir: "/ticket/render/acme.k8s/kubectl/overlays/prod/abc123",
		},
	}, fake, time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
	if len(fake.calls) != 1 || fake.calls[0].dir != "/ticket/render/acme.k8s/kubectl/overlays/prod/abc123" {
		t.Errorf("conftest call = %+v, want one call scoped to the render dir", fake.calls)
	}
}

func TestRunPolicyConftestRenderInputNoRenderDirIsError(t *testing.T) {
	c := standards.EffectiveCheck{
		ID: "infra.kubernetes.policy", Kind: "policy", Tool: "conftest", Severity: "must", Input: "render",
		Config: "policy", Pack: packWithConfigDir("policy", map[string]string{"labels.rego": "package main\n"}),
	}
	runner, _ := check.ForKind("policy")
	_, err := runner.Run(context.Background(), check.RunInput{
		Scratch: t.TempDir(),
		Check:   c,
		Target:  check.Target{Key: "acme.k8s:overlays/prod", Addon: "kubectl", Repo: "acme.k8s", SourceDir: "/repo/overlays/prod"},
	}, exectest.New(t), time.Second)
	if err == nil {
		t.Fatal("Run: want an error when input: render has no RenderDir, got nil")
	}
}

func TestRunPolicyUnknownToolIsError(t *testing.T) {
	c := standards.EffectiveCheck{ID: "x", Kind: "policy", Tool: "opa", Severity: "must"}
	runner, _ := check.ForKind("policy")
	_, err := runner.Run(context.Background(), check.RunInput{
		Scratch: t.TempDir(),
		Check:   c, Target: check.Target{Key: "a:b", SourceDir: "/repo"},
	}, exectest.New(t), time.Second)
	if err == nil {
		t.Fatal("Run: want an error for an unrecognized policy tool, got nil")
	}
}

// conftestCall is one recorded call through conftestExecer.
type conftestCall struct {
	tool string
	args []string
	dir  string
}

// conftestExecer wraps exectest.Scripted, answering any call to tool
// (default "conftest" when unset) with a fixed output regardless of
// the materialized policy dir's own (test-run-specific,
// deterministic-but-unpredictable-to-the-test) temp path — every other
// tool call still goes through the real Scripted and must be
// registered normally.
type conftestExecer struct {
	*exectest.Scripted
	out   exec.Output
	tool  string
	calls []conftestCall
}

func (c *conftestExecer) Run(ctx context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	if err := exectest.Refused(tool, args); err != nil {
		return exec.Output{}, err // what the real Runner answers (phase 54)
	}
	want := c.tool
	if want == "" {
		want = "conftest"
	}
	c.calls = append(c.calls, conftestCall{tool: tool, args: args, dir: opts.Dir})
	if tool == want {
		return c.out, nil
	}
	return c.Scripted.Run(ctx, tool, args, opts)
}
