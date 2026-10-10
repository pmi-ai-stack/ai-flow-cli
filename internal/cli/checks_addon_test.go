package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// writeRenderDriftFixture writes home/key/render-drift-results.json
// naming exactly one target at hash — enough for the addon-scoped
// "policy"/"scan" dispatch (phase 25) to locate that target's already
// rendered, redacted directory without this test needing to run `ops
// render`/`ops drift` for real.
func writeRenderDriftFixture(t *testing.T, home, key, targetKey, hash string) {
	t.Helper()
	artifact := renderDriftArtifact{Targets: []opsDriftResultTarget{{
		Drift: ops.DriftNotApplicable, Resources: []string{},
		Hash:       hash,
		Target:     targetKey,
		Validation: []ops.Finding{},
	}}}
	if _, err := handoff.WriteArtifact(filepath.Join(home, key), "render-drift-results.json", "render-drift-results", artifact); err != nil {
		t.Fatalf("write render-drift-results.json: %v", err)
	}
}

// TestStandardsCheckAddonScopedPolicyRunsAgainstRendering is this
// phase's own regression test for two things at once: (1) that a
// repo-layer "policy" check narrowed by applies_to.addons actually
// becomes part of the resolution once ops-targets.json names that
// addon (internal/cli/standards_resolve.go's Addons fix — before it,
// e.Addons stayed permanently nil and this check could never apply);
// and (2) that the addon-scoped dispatch runs it once per changed
// target, against the target's own already-written rendering (not the
// repository as a whole), locating that rendering via
// render-drift-results.json's own recorded hash.
func TestStandardsCheckAddonScopedPolicyRunsAgainstRendering(t *testing.T) {
	home, repoDir := standardsTicket(t, "PROJ-9")
	repoPack(t, repoDir, `{
  "schema_version": 1, "id": "acme.billing.api", "version": "0.1.0",
  "sections": [],
  "checks": [
    { "id": "repo.infra.policy", "kind": "policy", "tool": "conftest", "config": "checks/policy", "severity": "must", "input": "render",
      "applies_to": { "addons": ["terraform"] } }
  ]
}`)
	writeFileTest(t, filepath.Join(repoDir, ".ai-flow", "standards", "checks", "policy", "naming.rego"), "package main\n")

	targetKey := "acme.billing.api:infra/prod"
	hash := "abc123deadbeef"
	writeOpsTargetsFixture(t, home, "PROJ-9", []ops.Target{
		{Addon: "terraform", Repo: "acme.billing.api", Path: "infra/prod", Scope: ops.ScopeDiff, Changed: new(true)},
	})
	writeRenderDriftFixture(t, home, "PROJ-9", targetKey, hash)

	policyDir := filepath.Join(os.TempDir(), "ai-flow-check-policy-repo.infra.policy")
	renderDir := filepath.Join(home, "PROJ-9", "render", "acme.billing.api", "terraform", "infra", "prod", hash)

	// The embedded reference pack (the org layer, since standardsTicket
	// configures no config.standards.sources) now also declares its own
	// infra.terraform.policy/infra.terraform.scan checks (this phase's
	// own reference-pack content), narrowed by applies_to.addons:
	// ["terraform"] — the exact same addon this test's own OPS target
	// names, so both fire against it too, alongside the repo-layer
	// check under test. Both are scripted to a clean, empty response
	// (input: "source", so they scan sourceDir, not renderDir).
	embeddedTFPolicyDir := filepath.Join(os.TempDir(), "ai-flow-check-policy-infra.terraform.policy")
	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`).
		On("conftest", []string{"test", "--policy", policyDir, "--output", "json", "."},
			exec.Output{Stdout: `[{"filename":"main.tf","namespace":"main","failures":[{"msg":"missing required tag"}],"warnings":[]}]`}).
		On("conftest", []string{"test", "--policy", embeddedTFPolicyDir, "--output", "json", "."}, exec.Output{}).
		On("checkov", []string{"--directory", ".", "--output", "json", "--compact", "--quiet"}, exec.Output{})
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	env := runChecks(t, 0, "standards", "check", "PROJ-9", "--home", home)

	var saw bool
	for _, f := range env.Result.Findings {
		if f.CheckID == "repo.infra.policy" {
			saw = true
			if f.Severity != "must" {
				t.Errorf("finding severity = %q, want must (the check's own declared severity — a conftest failure has no graded severity of its own)", f.Severity)
			}
			if f.Status != "open" {
				t.Errorf("finding status = %q, want open", f.Status)
			}
		}
	}
	if !saw {
		t.Fatalf("findings = %+v, want one from repo.infra.policy", env.Result.Findings)
	}

	var calledWithRenderDir bool
	for _, c := range x.Calls {
		if c.Tool == "conftest" && c.Dir == renderDir {
			calledWithRenderDir = true
		}
	}
	if !calledWithRenderDir {
		t.Errorf("conftest calls = %+v, want one scoped to the target's rendering %s", x.Calls, renderDir)
	}
}

// TestStandardsCheckAddonScopedPolicySkipsWithoutOpsTargets proves the
// tolerant-of-absence half: an ordinary, non-infra ticket (no
// ops-targets.json at all) never even attempts to dispatch the
// addon-scoped check, and every other check still runs normally.
func TestStandardsCheckAddonScopedPolicySkipsWithoutOpsTargets(t *testing.T) {
	home, repoDir := standardsTicket(t, "PROJ-10")
	repoPack(t, repoDir, `{
  "schema_version": 1, "id": "acme.billing.api", "version": "0.1.0",
  "sections": [],
  "checks": [
    { "id": "repo.infra.policy", "kind": "policy", "tool": "conftest", "config": "checks/policy", "severity": "must", "input": "render",
      "applies_to": { "addons": ["terraform"] } }
  ]
}`)
	writeFileTest(t, filepath.Join(repoDir, ".ai-flow", "standards", "checks", "policy", "naming.rego"), "package main\n")

	x := scriptEmbeddedGoChecks(t, `{"Issues":[]}`)
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	env := runChecks(t, 0, "standards", "check", "PROJ-10", "--home", home)
	for _, f := range env.Result.Findings {
		if f.CheckID == "repo.infra.policy" {
			t.Fatalf("findings = %+v, want no repo.infra.policy finding without any OPS target", env.Result.Findings)
		}
	}
	for _, c := range x.Calls {
		if c.Tool == "conftest" {
			t.Errorf("unexpected conftest call %+v; no OPS target exists for this ticket", c)
		}
	}
}
