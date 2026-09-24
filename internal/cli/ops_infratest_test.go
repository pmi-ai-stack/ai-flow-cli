package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// realTerraformTargetTicket builds a ticket whose ops-targets.json
// names one changed target bound to the real "terraform" addon
// (unlike ops_render_test.go's own terraformTargetTicket, which
// deliberately uses "no-such-addon" to stay a harmless no-op — this
// phase's own tests need the real addon's real Harnesses/Render/DryRun
// path) and whose repository checkout at infra/prod carries a
// provider-free module (no init network access needed — the same
// shape hand-verified against a real `terraform test -json` run in
// this session, see the phase-24 plan's §0) plus one .tftest.hcl file.
func realTerraformTargetTicket(t *testing.T, key string) (home, repoDir string) {
	t.Helper()
	home = t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	repoDir = t.TempDir()
	moduleDir := filepath.Join(repoDir, "infra", "prod")
	if err := os.MkdirAll(moduleDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mainTF := "variable \"x\" {\n  type    = number\n  default = 2\n}\n\noutput \"doubled\" {\n  value = var.x * 2\n}\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(mainTF), 0o600); err != nil {
		t.Fatalf("write main.tf: %v", err)
	}
	passingTest := "run \"doubles_correctly\" {\n  command = plan\n  assert {\n    condition     = output.doubled == 4\n    error_message = \"bad\"\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "main.tftest.hcl"), []byte(passingTest), 0o600); err != nil {
		t.Fatalf("write main.tftest.hcl: %v", err)
	}
	writeStubInstructions(t, repoDir)

	ticketDir := filepath.Join(home, key)
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.infra", LocalPath: repoDir, InstructionsFile: "AGENTS.md"},
	})
	changed := true
	targets := ops.DetectResult{
		AppFiles: []string{},
		Targets:  []ops.Target{{Addon: "terraform", Repo: "acme.infra", Path: "infra/prod", Scope: ops.ScopeDiff, Changed: &changed, Files: []string{}}},
	}
	if _, err := handoff.WriteArtifact(ticketDir, "ops-targets.json", "ops-targets", targets); err != nil {
		t.Fatalf("write ops-targets.json: %v", err)
	}
	return home, repoDir
}

// requireRealTerraform skips the calling test when no terraform binary
// is on PATH — this file's own tests run a real terraform test (a
// provider-free module, no network access), not a scripted fixture,
// mirroring phases 21/22's own -tags ops posture but without the build
// tag, since it costs nothing extra and exercises the real invocation
// shape in this development environment where terraform happens to be
// available.
func requireRealTerraform(t *testing.T) {
	t.Helper()
	if _, found := exec.Which("terraform"); !found {
		t.Skip("terraform not found on PATH")
	}
}

type opsTestDetectEnvelope struct {
	OK     bool `json:"ok"`
	Result struct {
		Harnesses []opsTestDetectEntry `json:"harnesses"`
	} `json:"result"`
}

func TestOpsTestDetectListsRealHarness(t *testing.T) {
	requireRealTerraform(t)
	home, _ := realTerraformTargetTicket(t, "PROJ-1")

	stdout, _, ec := runRoot(t, "--home", home, "ops", "test", "detect", "PROJ-1")
	if ec != 0 {
		t.Fatalf("exitCode = %d\n%s", ec, stdout.String())
	}
	var env opsTestDetectEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Harnesses) != 1 {
		t.Fatalf("harnesses = %+v, want exactly one", env.Result.Harnesses)
	}
	h := env.Result.Harnesses[0]
	if h.Target != "acme.infra:infra/prod" || h.Harness != "infra/prod/main.tftest.hcl" || h.Kind != "unit" || h.Tool != "terraform-test" || !h.WillRun {
		t.Errorf("harness entry = %+v, unexpected", h)
	}
}

type opsTestRunEnvelope struct {
	OK    bool `json:"ok"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
	Result opsTestRunResult `json:"result"`
}

func decodeOpsTestRun(t *testing.T, stdout []byte) opsTestRunEnvelope {
	t.Helper()
	var env opsTestRunEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	return env
}

func TestOpsTestRunExecutesRealTerraformTestAndPersists(t *testing.T) {
	requireRealTerraform(t)
	home, _ := realTerraformTargetTicket(t, "PROJ-1")

	stdout, _, ec := runRoot(t, "--home", home, "ops", "test", "run", "PROJ-1")
	if ec != 0 {
		t.Fatalf("exitCode = %d\n%s", ec, stdout.String())
	}
	env := decodeOpsTestRun(t, stdout.Bytes())
	if len(env.Result.Harnesses) != 1 {
		t.Fatalf("harnesses = %+v, want exactly one", env.Result.Harnesses)
	}
	got := env.Result.Harnesses[0]
	if got.Total != 1 || got.Passed != 1 || got.Failed != 0 {
		t.Fatalf("entry = %+v, want a real, passing terraform test result", got)
	}

	// infra-test-results.json was written (--ref branch, the default,
	// per plan design decision 10) and the raw report exists on disk.
	data, err := handoff.ReadArtifact(filepath.Join(home, "PROJ-1", "infra-test-results.json"))
	if err != nil || data == nil {
		t.Fatalf("infra-test-results.json: data=%v err=%v", data, err)
	}
	var artifact infraTestArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode infra-test-results.json: %v", err)
	}
	if len(artifact.Targets) != 1 || artifact.Targets[0].Failed != 0 || artifact.Targets[0].Total != 1 {
		t.Fatalf("infra-test-results.json targets = %+v", artifact.Targets)
	}
	reportPath := filepath.Join(home, "PROJ-1", artifact.Targets[0].Report)
	if _, err := os.Stat(reportPath); err != nil {
		t.Errorf("report file %s does not exist: %v", reportPath, err)
	}
}

func TestOpsTestRunHarnessFlagNarrowsAndPreservesOtherEntries(t *testing.T) {
	requireRealTerraform(t)
	home, _ := realTerraformTargetTicket(t, "PROJ-1")
	ticketDir := filepath.Join(home, "PROJ-1")

	// Seed an existing entry for a *different* harness — the merge must
	// leave it untouched (plan design decision 18).
	seed := infraTestArtifact{Targets: []infraTestEntry{
		{Target: "acme.infra:infra/prod", Harness: "infra/prod/other.tftest.hcl", Kind: "unit", Tool: "terraform-test", Total: 1, Passed: 1, Failures: []string{}, Report: "infra-test-reports/seed.log"},
	}}
	if _, err := handoff.WriteArtifact(ticketDir, "infra-test-results.json", "infra-test-results", seed); err != nil {
		t.Fatalf("seed infra-test-results.json: %v", err)
	}

	stdout, _, ec := runRoot(t, "--home", home, "ops", "test", "run", "PROJ-1", "--harness", "infra/prod/main.tftest.hcl")
	if ec != 0 {
		t.Fatalf("exitCode = %d\n%s", ec, stdout.String())
	}

	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "infra-test-results.json"))
	if err != nil || data == nil {
		t.Fatalf("infra-test-results.json: data=%v err=%v", data, err)
	}
	var artifact infraTestArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(artifact.Targets) != 2 {
		t.Fatalf("targets = %+v, want the seeded entry preserved plus the new one", artifact.Targets)
	}
}

func TestOpsTestRunUnknownHarnessFlagIsUsageError(t *testing.T) {
	requireRealTerraform(t)
	home, _ := realTerraformTargetTicket(t, "PROJ-1")
	stdout, _, ec := runRoot(t, "--home", home, "ops", "test", "run", "PROJ-1", "--harness", "nope.tftest.hcl")
	if ec != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", ec, stdout.String())
	}
}

func TestOpsTestRunRefBaseDoesNotPersist(t *testing.T) {
	requireRealTerraform(t)
	home, repoDir := realTerraformTargetTicket(t, "PROJ-1")

	// git prepare's own base worktree is what `--ref base` resolves
	// against; without one, resolveTargetSourceDir refuses with
	// missing_artifact — this test only needs to prove infra-test-results.json
	// stays absent on a --ref base run, so a failing base-ref render
	// (no base worktree) already proves it trivially, without needing a
	// real git repository fixture.
	_ = repoDir
	stdout, _, ec := runRoot(t, "--home", home, "ops", "test", "run", "PROJ-1", "--ref", "base")
	if ec == 0 {
		t.Fatalf("exitCode = 0, want a failure (no base worktree exists)\n%s", stdout.String())
	}

	data, err := handoff.ReadArtifact(filepath.Join(home, "PROJ-1", "infra-test-results.json"))
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if data != nil {
		t.Error("infra-test-results.json was written by a --ref base run; want it to stay absent")
	}
}

func TestOpsTestTeardownNoOpsWithNoCluster(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	stdout, _, ec := runRoot(t, "--home", home, "ops", "test", "teardown", "PROJ-1")
	if ec != 0 {
		t.Fatalf("exitCode = %d, want 0 (a missing kind binary/cluster is tolerated)\n%s", ec, stdout.String())
	}
}

// TestStageCompleteInfraTestRefusesOnFailureThenPassesAfterFix pins
// checkNoInfraTestFailure's own wiring into `stage complete KEY
// infra-test` (plan design decision 11) — mirroring
// TestStageCompleteRollbackPlanCheck's exact shape for
// checkRollbackPlanCheck.
func TestStageCompleteInfraTestRefusesOnFailureThenPassesAfterFix(t *testing.T) {
	// No requireRealTerraform guard: this test never runs `ops test
	// run` (it hand-writes infra-test-results.json directly, mirroring
	// TestStageCompleteRollbackPlanCheck's own precedent), and
	// terraform.Harnesses (needed for `stage start infra-test`'s own
	// ShouldRun rule) calls no subprocess at all — only
	// realTerraformTargetTicket's *name*, not its behaviour here, is
	// about terraform.
	home, _ := realTerraformTargetTicket(t, "PROJ-1")
	ticketDir := filepath.Join(home, "PROJ-1")

	env := runStage(t, home, 0, "start", "PROJ-1", "infra-test")
	if env.Result.State != "running" {
		t.Fatalf("stage start infra-test: state = %s, want running", env.Result.State)
	}

	failing := infraTestArtifact{Targets: []infraTestEntry{
		{Target: "acme.infra:infra/prod", Harness: "infra/prod/main.tftest.hcl", Kind: "unit", Tool: "terraform-test", Total: 1, Passed: 0, Failed: 1, Failures: []string{"doubles_correctly: bad"}, Report: "infra-test-reports/x.log"},
	}}
	if _, err := handoff.WriteArtifact(ticketDir, "infra-test-results.json", "infra-test-results", failing); err != nil {
		t.Fatalf("write infra-test-results.json: %v", err)
	}
	env = runStage(t, home, 6, "complete", "PROJ-1", "infra-test")
	if env.Error == nil || env.Error.Code != "invalid_result" {
		t.Fatalf("error = %+v, want invalid_result", env.Error)
	}

	passing := infraTestArtifact{Targets: []infraTestEntry{
		{Target: "acme.infra:infra/prod", Harness: "infra/prod/main.tftest.hcl", Kind: "unit", Tool: "terraform-test", Total: 1, Passed: 1, Failed: 0, Failures: []string{}, Report: "infra-test-reports/x.log"},
	}}
	if _, err := handoff.WriteArtifact(ticketDir, "infra-test-results.json", "infra-test-results", passing); err != nil {
		t.Fatalf("write infra-test-results.json: %v", err)
	}
	env = runStage(t, home, 0, "complete", "PROJ-1", "infra-test")
	if env.Result.State != "pass" {
		t.Errorf("state = %s, want pass once every harness passes", env.Result.State)
	}
}

func TestOpsTestRunSimulateDoesNothing(t *testing.T) {
	requireRealTerraform(t)
	home, _ := realTerraformTargetTicket(t, "PROJ-1")
	stdout, _, ec := runRoot(t, "--home", home, "--simulate", "ops", "test", "run", "PROJ-1")
	if ec != 0 {
		t.Fatalf("exitCode = %d\n%s", ec, stdout.String())
	}
	data, err := handoff.ReadArtifact(filepath.Join(home, "PROJ-1", "infra-test-results.json"))
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if data != nil {
		t.Error("--simulate wrote infra-test-results.json, want no side effect")
	}
}
