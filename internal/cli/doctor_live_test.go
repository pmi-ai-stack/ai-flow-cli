package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// writeKubeEnvConfig writes home/config.json with one kube-bound
// environment ("prod", context "prod-ro") and one without ("dev") —
// doctorProbeLiveEnvironments's own precondition for having anything
// to probe.
func writeKubeEnvConfig(t *testing.T, home string) {
	t.Helper()
	const cfg = `{
  "schema_version": 1,
  "tracker": { "provider": "jira", "base_url": "http://127.0.0.1:0", "statuses": { "todo": ["To Do"], "in_progress": "In Progress", "in_review": "In Review" }, "bug_types": ["Bug"], "comment_limit": 0 },
  "host": { "provider": "github", "default_base_branch": "main", "draft_prs": false },
  "routing": { "project_key_map": {}, "keyword_to_repo": {}, "teams": {} },
  "defaults": { "worktree": false, "viewport": "1440x900", "mutation_threshold": 80, "mutation_max_rounds": 3, "mutation_timeout_minutes": 20, "test_timeout_minutes": 20, "guard_idle_hours": 8 },
  "standards": { "sources": [], "guidance_budget_tokens": 6000 },
  "ops": {
    "environments": {
      "dev": { "tier": "nonprod" },
      "prod": { "tier": "prod", "kube": { "context": "prod-ro" } }
    },
    "target_map": {}, "default_environment": "dev", "risk_gate": 40,
    "risk": { "action_weight": {}, "kind_weight": {}, "tier_multiplier": {}, "drift_penalty": 5, "offline_penalty": 10 },
    "protected_kinds": [], "e2e": { "enabled": false, "sandbox_environment": "" }, "tool_timeout_minutes": 1
  }
}`
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// TestDoctorLiveEnvironmentsOnlyProbesKubeBound proves "dev" (no kube
// binding) is skipped entirely and only "prod" is probed.
func TestDoctorLiveEnvironmentsOnlyProbesKubeBound(t *testing.T) {
	home := t.TempDir()
	writeKubeEnvConfig(t, home)

	x := exectest.New(t)
	x.On("kubectl", []string{"auth", "can-i", "--list", "--context", "prod-ro"}, exec.Output{Stdout: "get\tpods\nlist\tpods\n"})
	useScriptedExecer(t, x)

	got := runDoctor(t, home)
	if len(got.LiveEnvironments) != 1 {
		t.Fatalf("LiveEnvironments = %+v, want exactly one (prod)", got.LiveEnvironments)
	}
	le := got.LiveEnvironments[0]
	if le.Environment != "prod" || le.Context != "prod-ro" || le.Tier != "prod" {
		t.Errorf("LiveEnvironments[0] = %+v, want environment=prod context=prod-ro tier=prod", le)
	}
	if le.Reachable == nil || !*le.Reachable {
		t.Fatalf("Reachable = %v, want true", le.Reachable)
	}
	if le.ReadOnly == nil || !*le.ReadOnly {
		t.Errorf("ReadOnly = %v, want true (no mutating verb in the scripted output)", le.ReadOnly)
	}
}

// TestDoctorLiveEnvironmentsFlagsMutatingContext proves the keyword
// heuristic: a "create"/"update"/... verb in can-i --list's own output
// reports read_only: false and a warning.
func TestDoctorLiveEnvironmentsFlagsMutatingContext(t *testing.T) {
	home := t.TempDir()
	writeKubeEnvConfig(t, home)

	x := exectest.New(t)
	x.On("kubectl", []string{"auth", "can-i", "--list", "--context", "prod-ro"}, exec.Output{Stdout: "get\tpods\ncreate\tpods\n"})
	useScriptedExecer(t, x)

	got := runDoctor(t, home)
	if len(got.LiveEnvironments) != 1 {
		t.Fatalf("LiveEnvironments = %+v, want exactly one", got.LiveEnvironments)
	}
	le := got.LiveEnvironments[0]
	if le.ReadOnly == nil || *le.ReadOnly {
		t.Errorf("ReadOnly = %v, want false (the scripted output grants create)", le.ReadOnly)
	}
}

// TestDoctorLiveEnvironmentsToolFailedIsAFindingNotAnError proves a
// probe failure (kubectl absent, cluster unreachable) never fails the
// doctor command itself.
func TestDoctorLiveEnvironmentsToolFailedIsAFindingNotAnError(t *testing.T) {
	home := t.TempDir()
	writeKubeEnvConfig(t, home)

	x := exectest.New(t)
	x.OnFail("kubectl", []string{"auth", "can-i", "--list", "--context", "prod-ro"}, &fakeToolFailedError{})
	useScriptedExecer(t, x)

	stdout, _, exitCode := runRoot(t, "doctor", "--home", home)
	if exitCode != 0 {
		t.Fatalf("doctor: exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	got := runDoctor(t, home)
	if len(got.LiveEnvironments) != 1 {
		t.Fatalf("LiveEnvironments = %+v, want exactly one", got.LiveEnvironments)
	}
	le := got.LiveEnvironments[0]
	if le.Reachable == nil || *le.Reachable {
		t.Errorf("Reachable = %v, want false", le.Reachable)
	}
	if le.Error == "" {
		t.Error("Error is empty, want a message")
	}
	if le.ReadOnly != nil {
		t.Errorf("ReadOnly = %v, want nil (no output to read a verdict from)", le.ReadOnly)
	}
}

// TestDoctorLiveEnvironmentsNoKubeBindingIsEmpty proves the default
// template (only "dev", no kube binding) probes nothing.
func TestDoctorLiveEnvironmentsNoKubeBindingIsEmpty(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	got := runDoctor(t, home)
	if len(got.LiveEnvironments) != 0 {
		t.Errorf("LiveEnvironments = %+v, want none", got.LiveEnvironments)
	}
}

type fakeToolFailedError struct{}

func (e *fakeToolFailedError) Error() string { return "exit status 127: kubectl: command not found" }
