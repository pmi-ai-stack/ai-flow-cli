package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// TestNeedsInputForEnvAmbiguous is a direct unit test of the mapping
// needsInputFor's new case adds: no addon is registered in
// DefaultRegistry before phase 21, so env_ambiguous cannot actually
// fire through a real `ops detect KEY` run today (Bind is only called
// per detected target, and there are none) — internal/ops/envs's own
// TestBindAmbiguousWhenTwoDifferentConventionsMatchTwoDifferentEnvironments
// already proves Bind itself produces the error; this proves the CLI
// maps it to needs_input correctly once phase 21+ makes it reachable.
func TestNeedsInputForEnvAmbiguous(t *testing.T) {
	err := &envelope.Error{Code: "env_ambiguous", Details: []string{"dev", "prod"}}
	got := needsInputFor(err, "PROJ-1")
	if got == nil {
		t.Fatal("needsInputFor = nil, want a NeedsInput for env_ambiguous")
	}
	if len(got.Options) != 2 || got.Options[0] != "dev" || got.Options[1] != "prod" {
		t.Errorf("Options = %v, want [dev prod]", got.Options)
	}
	if got.ResumeWith != "ops detect PROJ-1" {
		t.Errorf("ResumeWith = %q, want %q", got.ResumeWith, "ops detect PROJ-1")
	}
}

// TestNeedsInputForLiveCredentialsMissing is a direct unit test of the
// phase-20 mapping: options are the spec's own two (provide, offline),
// resume_with is the literal `ops dry-run KEY --offline` a developer
// answering "offline" would run.
func TestNeedsInputForLiveCredentialsMissing(t *testing.T) {
	err := &envelope.Error{Code: "live_credentials_missing"}
	got := needsInputFor(err, "PROJ-1")
	if got == nil {
		t.Fatal("needsInputFor = nil, want a NeedsInput for live_credentials_missing")
	}
	if len(got.Options) != 2 || got.Options[0] != "provide" || got.Options[1] != "offline" {
		t.Errorf("Options = %v, want [provide offline]", got.Options)
	}
	if got.ResumeWith != "ops dry-run PROJ-1 --offline" {
		t.Errorf("ResumeWith = %q, want %q", got.ResumeWith, "ops dry-run PROJ-1 --offline")
	}
}

type opsTargetsFixture struct {
	AppFiles []string `json:"app_files"`
	Targets  []struct {
		Addon       string   `json:"addon"`
		Repo        string   `json:"repo"`
		Path        string   `json:"path"`
		Scope       string   `json:"scope"`
		Changed     *bool    `json:"changed"`
		Environment *string  `json:"environment"`
		Tier        *string  `json:"tier"`
		Files       []string `json:"files"`
	} `json:"targets"`
}

func readOpsTargets(t *testing.T, home, key string) opsTargetsFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, key, "ops-targets.json"))
	if err != nil {
		t.Fatalf("ReadFile ops-targets.json: %v", err)
	}
	var got opsTargetsFixture
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode ops-targets.json: %v\n%s", err, data)
	}
	return got
}

// TestStageStartPlanWritesOpsTargetsAtRepoScope is phase 18's own
// "Done when": `ops detect` at `plan` writes scope: repo with
// changed: null, and DefaultRegistry ships with zero addons today, so
// targets is empty — not a stub, the correct answer before phase 21.
func TestStageStartPlanWritesOpsTargetsAtRepoScope(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")
	runStage(t, home, 0, "start", "PROJ-1", "plan")

	got := readOpsTargets(t, home, "PROJ-1")
	if len(got.Targets) != 0 {
		t.Fatalf("Targets = %v, want empty (no addon registered before phase 21)", got.Targets)
	}
	if len(got.AppFiles) != 0 {
		t.Errorf("AppFiles = %v, want empty at scope repo (nothing changed yet)", got.AppFiles)
	}
}

// TestStageCompleteImplementWritesOpsTargetsAtDiffScope proves the
// implement-completion side of the wiring: scope moves to "diff" and
// app_files reflects repo.json's own in_scope_files once repo scope
// has run.
func TestStageCompleteImplementWritesOpsTargetsAtDiffScope(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")
	runStage(t, home, 0, "start", "PROJ-1", "plan")

	ticketDir := filepath.Join(home, "PROJ-1")
	setInScopeFiles(t, ticketDir, []string{"main.go", "README.md"})

	runStage(t, home, 0, "start", "PROJ-1", "implement")
	writeFixture(t, home, "PROJ-1", "test-results", "test-results.json")
	runStage(t, home, 0, "complete", "PROJ-1", "implement")

	got := readOpsTargets(t, home, "PROJ-1")
	if len(got.Targets) != 0 {
		t.Fatalf("Targets = %v, want empty (no addon registered before phase 21)", got.Targets)
	}
	want := []string{"main.go"} // README.md is documentation, excluded by detect.AppFiles.
	if len(got.AppFiles) != 1 || got.AppFiles[0] != want[0] {
		t.Errorf("AppFiles = %v, want %v", got.AppFiles, want)
	}
}

// setInScopeFiles rewrites repo.json's in_scope_files directly — the
// same effect `repo scope` has, without needing a real git checkout
// with a real diff against origin/main (this suite's fixture
// repositories are plain temp directories, not git repositories).
func setInScopeFiles(t *testing.T, ticketDir string, files []string) {
	t.Helper()
	path := filepath.Join(ticketDir, "repo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var artifact repoArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	artifact.InScopeFiles = files
	out, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestOpsAllowlistCommandListsOpsRows proves `ops allowlist` surfaces
// this phase's own new rows through the envelope, not just Table
// itself.
func TestOpsAllowlistCommandListsOpsRows(t *testing.T) {
	stdout, _, ec := runRoot(t, "ops", "allowlist")
	if ec != 0 {
		t.Fatalf("ops allowlist: exitCode = %d\n%s", ec, stdout.String())
	}
	var env struct {
		Result struct {
			Rows []struct {
				Tool  string `json:"tool"`
				Class string `json:"class"`
			} `json:"rows"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Rows) != len(exec.Table) {
		t.Errorf("ops allowlist returned %d rows, Table has %d", len(env.Result.Rows), len(exec.Table))
	}
	var sawTerraform bool
	for _, r := range env.Result.Rows {
		if r.Tool == "terraform" {
			sawTerraform = true
		}
	}
	if !sawTerraform {
		t.Error("ops allowlist did not list a terraform row")
	}
}

// TestOpsAddonsCommandListsTerraformAndKubectl proves `ops addons`
// reports the real registry buildOpsRegistry constructs — "terraform"
// then "kubectl," in registration order, from phase 22 on
// (internal/ops.DefaultRegistry() itself stays empty forever, by
// design; see its own doc comment). Renamed from
// TestOpsAddonsCommandListsTerraform (phase 21) now that a second
// addon is registered.
func TestOpsAddonsCommandListsTerraformAndKubectl(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	stdout, _, ec := runRoot(t, "--home", home, "ops", "addons")
	if ec != 0 {
		t.Fatalf("ops addons: exitCode = %d\n%s", ec, stdout.String())
	}
	var env struct {
		Result struct {
			Addons []struct {
				Name         string `json:"name"`
				Capabilities struct {
					Render, Validate, Drift, DryRun, Rollback, UnitTest, E2ETest bool
				} `json:"capabilities"`
			} `json:"addons"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Addons) != 2 || env.Result.Addons[0].Name != "terraform" || env.Result.Addons[1].Name != "kubectl" {
		t.Fatalf("ops addons = %+v, want exactly two, named terraform then kubectl", env.Result.Addons)
	}
	tf := env.Result.Addons[0].Capabilities
	if !tf.Render || !tf.Validate || !tf.Drift || !tf.DryRun || !tf.Rollback || !tf.UnitTest || !tf.E2ETest {
		t.Errorf("terraform capabilities = %+v, want every one true", tf)
	}
	kc := env.Result.Addons[1].Capabilities
	if !kc.Render || !kc.Validate || !kc.Drift || !kc.DryRun || !kc.Rollback || kc.UnitTest || !kc.E2ETest {
		t.Errorf("kubectl capabilities = %+v, want every one true except unit_test", kc)
	}
}
