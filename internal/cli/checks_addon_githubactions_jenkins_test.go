package cli

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// TestStandardsCheckGithubActionsPolicyAndScanFireAgainstRendering
// proves phase 27's own two new reference-pack entries
// (infra.github-actions.policy/infra.github-actions.scan) actually
// reach a real github-actions OPS target through the addon-scoped
// dispatch (internal/cli/checks.go), the same mechanism
// TestStandardsCheckAddonScopedPolicyRunsAgainstRendering already
// proved for terraform.
func TestStandardsCheckGithubActionsPolicyAndScanFireAgainstRendering(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-27")

	targetKey := "acme.billing.api:.github/workflows/ci.yml"
	hash := "ghaction123hash"
	writeOpsTargetsFixture(t, home, "PROJ-27", []ops.Target{
		{Addon: "github-actions", Repo: "acme.billing.api", Path: ".github/workflows/ci.yml", Scope: ops.ScopeDiff, Changed: new(true)},
	})
	writeRenderDriftFixture(t, home, "PROJ-27", targetKey, hash)

	renderDir := filepath.Join(home, "PROJ-27", "render", "acme.billing.api", "github-actions", ".github", "workflows", "ci.yml", hash)
	policyDir := filepath.Join(pinCheckScratch(t), "policy-infra.github-actions.policy")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`).
		On("conftest", []string{"test", "--policy", policyDir, "--output", "json", "."},
			exec.Output{Stdout: `[{"filename":"workflow.yml","namespace":"main","failures":[{"msg":"job \"build\" has no permissions"}],"warnings":[]}]`}).
		On("zizmor", []string{"--format", "json", "."}, exec.Output{
			Stdout: `[{"ident":"unpinned-uses","desc":"unpinned action reference","determinations":{"severity":"High"},"locations":[]}]`,
		}).
		On("checkov", []string{"--directory", ".", "--output", "json", "--compact", "--quiet"}, exec.Output{})
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	env := runChecks(t, 0, "standards", "check", "PROJ-27", "--home", home)

	var sawPolicy, sawScan bool
	for _, f := range env.Result.Findings {
		switch f.CheckID {
		case "infra.github-actions.policy":
			sawPolicy = true
		case "infra.github-actions.scan":
			sawScan = true
		}
	}
	if !sawPolicy {
		t.Errorf("findings = %+v, want one from infra.github-actions.policy", env.Result.Findings)
	}
	if !sawScan {
		t.Errorf("findings = %+v, want one from infra.github-actions.scan", env.Result.Findings)
	}

	var calledWithRenderDir bool
	for _, c := range x.Calls {
		if c.Tool == "zizmor" && c.Dir == renderDir {
			calledWithRenderDir = true
		}
	}
	if !calledWithRenderDir {
		t.Errorf("zizmor calls = %+v, want one scoped to the target's rendering %s", x.Calls, renderDir)
	}
}

// TestStandardsCheckJenkinsPolicyFiresAgainstRendering mirrors the
// github-actions test above for infra.jenkins.policy.
func TestStandardsCheckJenkinsPolicyFiresAgainstRendering(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-28")

	targetKey := "acme.billing.api:jobs/build.xml"
	hash := "jenkinshash456"
	writeOpsTargetsFixture(t, home, "PROJ-28", []ops.Target{
		{Addon: "jenkins", Repo: "acme.billing.api", Path: "jobs/build.xml", Scope: ops.ScopeDiff, Changed: new(true)},
	})
	writeRenderDriftFixture(t, home, "PROJ-28", targetKey, hash)

	policyDir := filepath.Join(pinCheckScratch(t), "policy-infra.jenkins.policy")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`).
		On("conftest", []string{"test", "--policy", policyDir, "--output", "json", "."},
			exec.Output{Stdout: `[{"filename":"build.xml","namespace":"main","failures":[{"msg":"job config has no <assignedNode>"}],"warnings":[]}]`}).
		On("checkov", []string{"--directory", ".", "--output", "json", "--compact", "--quiet"}, exec.Output{})
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	env := runChecks(t, 0, "standards", "check", "PROJ-28", "--home", home)

	var saw bool
	for _, f := range env.Result.Findings {
		if f.CheckID == "infra.jenkins.policy" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("findings = %+v, want one from infra.jenkins.policy", env.Result.Findings)
	}
}
