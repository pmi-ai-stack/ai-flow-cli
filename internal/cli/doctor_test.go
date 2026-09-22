package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff/handofftest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// useScriptedJira swaps newJiraTransport for the duration of the test —
// the Jira-side twin of pr_test.go's useScriptedGitHub, sharing the
// same useScriptedTransport body.
func useScriptedJira(t *testing.T, handler func(r *http.Request) (*http.Response, error)) {
	t.Helper()
	useScriptedTransport(t, &newJiraTransport, handler)
}

func runDoctor(t *testing.T, home string) doctorResult {
	t.Helper()
	stdout, _, exitCode := runRoot(t, "doctor", "--home", home)
	if exitCode != 0 {
		t.Fatalf("doctor: exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result doctorResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	return env.Result
}

func TestDoctor_NoConfig(t *testing.T) {
	home := t.TempDir()
	got := runDoctor(t, home)

	if got.Config.Exists {
		t.Error("Config.Exists = true, want false")
	}
	if got.Config.Valid {
		t.Error("Config.Valid = true, want false")
	}
}

func TestDoctor_InvalidConfig(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	got := runDoctor(t, home)

	if !got.Config.Exists {
		t.Error("Config.Exists = false, want true")
	}
	if got.Config.Valid {
		t.Error("Config.Valid = true, want false")
	}
	if len(got.Config.Errors) == 0 {
		t.Error("Config.Errors is empty, want at least one")
	}
}

// writeValidDoctorConfig writes a config.json that satisfies every
// config.Validate rule — writeRoutingTestConfig's own fixture leaves
// ops.default_environment empty (fine for the routing/git/pr tests it
// was built for, which never call Validate), so this test needs its
// own.
func writeValidDoctorConfig(t *testing.T, home string) {
	t.Helper()
	cfg := `{
  "schema_version": 1,
  "tracker": { "provider": "jira", "base_url": "https://example.atlassian.net", "statuses": { "todo": ["To Do"], "in_progress": "In Progress", "in_review": "In Review" }, "bug_types": ["Bug"], "comment_limit": 5 },
  "host": { "provider": "github", "default_base_branch": "main", "draft_prs": false },
  "routing": { "project_key_map": {}, "keyword_to_repo": {}, "teams": {} },
  "defaults": { "worktree": false, "viewport": "1440x900", "mutation_threshold": 80, "mutation_max_rounds": 3, "mutation_timeout_minutes": 20, "test_timeout_minutes": 20, "guard_idle_hours": 8 },
  "standards": { "sources": [], "guidance_budget_tokens": 6000 },
  "ops": { "environments": { "dev": { "tier": "nonprod" } }, "target_map": {}, "default_environment": "dev", "risk_gate": 40, "risk": { "action_weight": {}, "kind_weight": {}, "tier_multiplier": {}, "drift_penalty": 5, "offline_penalty": 10 }, "protected_kinds": [], "e2e": { "enabled": false, "sandbox_environment": "" }, "tool_timeout_minutes": 30 }
}`
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

func TestDoctor_ValidConfig(t *testing.T) {
	home := t.TempDir()
	writeValidDoctorConfig(t, home)

	got := runDoctor(t, home)
	if !got.Config.Exists || !got.Config.Valid {
		t.Errorf("Config = %+v, want Exists=true Valid=true", got.Config)
	}
	if len(got.Config.Errors) != 0 {
		t.Errorf("Config.Errors = %v, want none", got.Config.Errors)
	}
}

func TestDoctor_EnvVarsNamesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "")
	t.Setenv("AI_FLOW_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	got := runDoctor(t, home)
	set := map[string]bool{}
	for _, v := range got.EnvVars {
		set[v.Name] = v.Set
	}
	if !set["AI_FLOW_JIRA_EMAIL"] {
		t.Error("AI_FLOW_JIRA_EMAIL: Set = false, want true")
	}
	if set["AI_FLOW_JIRA_TOKEN"] {
		t.Error("AI_FLOW_JIRA_TOKEN: Set = true, want false")
	}
	// The value itself must never leak into the result.
	for _, v := range got.EnvVars {
		if v.Name != "AI_FLOW_JIRA_EMAIL" {
			continue
		}
		raw, _ := json.Marshal(v)
		if string(raw) != `{"name":"AI_FLOW_JIRA_EMAIL","set":true}` {
			t.Errorf("env var entry leaked more than name+set: %s", raw)
		}
	}
}

func TestDoctor_TrackerCredsAbsent(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false)

	got := runDoctor(t, home)
	if got.Tracker.CredsSet {
		t.Error("Tracker.CredsSet = true, want false")
	}
	if got.Tracker.Reachable != nil {
		t.Errorf("Tracker.Reachable = %v, want nil", *got.Tracker.Reachable)
	}
}

func TestDoctor_TrackerReachableAndStatusesMatch(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	useScriptedJira(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/rest/api/2/myself":
			return jsonResponse(200, `{"accountId":"1"}`), nil
		case "/rest/api/2/status":
			return jsonResponse(200, `[{"name":"To Do"},{"name":"In Progress"},{"name":"In Review"}]`), nil
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
			return nil, nil
		}
	})

	got := runDoctor(t, home)
	if !got.Tracker.CredsSet {
		t.Fatal("Tracker.CredsSet = false, want true")
	}
	if got.Tracker.Reachable == nil || !*got.Tracker.Reachable {
		t.Fatalf("Tracker.Reachable = %v, want true", got.Tracker.Reachable)
	}
	if len(got.Tracker.MissingStatuses) != 0 {
		t.Errorf("Tracker.MissingStatuses = %v, want none", got.Tracker.MissingStatuses)
	}
}

func TestDoctor_TrackerMissingConfiguredStatus(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false) // configures "To Do", "In Progress", "In Review"
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	useScriptedJira(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/rest/api/2/myself":
			return jsonResponse(200, `{"accountId":"1"}`), nil
		case "/rest/api/2/status":
			// "In Review" is missing from the tracker's real workflow.
			return jsonResponse(200, `[{"name":"To Do"},{"name":"In Progress"}]`), nil
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
			return nil, nil
		}
	})

	got := runDoctor(t, home)
	if len(got.Tracker.MissingStatuses) != 1 || got.Tracker.MissingStatuses[0] != "In Review" {
		t.Errorf("Tracker.MissingStatuses = %v, want [\"In Review\"]", got.Tracker.MissingStatuses)
	}
}

func TestDoctor_TrackerUnreachable(t *testing.T) {
	home := t.TempDir()
	localBase := t.TempDir()
	writeRoutingTestConfig(t, home, localBase, false)
	t.Setenv("AI_FLOW_JIRA_EMAIL", "a@b.com")
	t.Setenv("AI_FLOW_JIRA_TOKEN", "tok")

	useScriptedJira(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(401, `{"errorMessages":["nope"]}`), nil
	})

	got := runDoctor(t, home)
	if got.Tracker.Reachable == nil || *got.Tracker.Reachable {
		t.Fatalf("Tracker.Reachable = %v, want false", got.Tracker.Reachable)
	}
	if got.Tracker.Error == "" {
		t.Error("Tracker.Error is empty, want a message")
	}
}

func TestDoctor_HostCredsAbsentThenReachable(t *testing.T) {
	home := t.TempDir()

	got := runDoctor(t, home)
	if got.Host.CredsSet {
		t.Error("Host.CredsSet = true, want false")
	}

	t.Setenv("AI_FLOW_GITHUB_TOKEN", "tok")
	useScriptedGitHub(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/user" {
			return jsonResponse(200, `{"login":"octocat"}`), nil
		}
		t.Fatalf("unexpected request %s", r.URL.Path)
		return nil, nil
	})

	got = runDoctor(t, home)
	if !got.Host.CredsSet {
		t.Fatal("Host.CredsSet = false, want true")
	}
	if got.Host.Reachable == nil || !*got.Host.Reachable {
		t.Fatalf("Host.Reachable = %v, want true", got.Host.Reachable)
	}
}

func TestDoctor_GitAndRunnerPresence(t *testing.T) {
	dir := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil { // throwaway PATH fixture
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", dir)

	home := t.TempDir()
	got := runDoctor(t, home)

	if !got.Git.Found {
		t.Error("Git.Found = false, want true")
	}
	for _, rt := range got.TestRunners {
		if rt.Name == "go" && rt.Found {
			t.Error("TestRunners[go].Found = true against a sandboxed PATH with only git on it")
		}
	}
}

func TestDoctor_NoActiveTicket(t *testing.T) {
	home := t.TempDir()
	got := runDoctor(t, home)
	if got.Active != nil {
		t.Errorf("Active = %+v, want nil", got.Active)
	}
}

func TestDoctor_ActiveStaleAndVersionMismatch(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	handofftest.FixedClock(t, now)

	stale := now.Add(-9 * time.Hour) // past the default 8h guard_idle_hours
	active := &handoff.Active{
		Key: "PROJ-1", Stage: "implement", RepoRoots: []string{}, ToolConfigFiles: []string{},
		AllowlistHash: "not-the-real-hash", LastEventAt: stale.Format(time.RFC3339),
	}
	if _, err := handoff.SaveActive(home, active); err != nil {
		t.Fatalf("SaveActive: %v", err)
	}

	got := runDoctor(t, home)
	if got.Active == nil {
		t.Fatal("Active = nil, want a report")
	}
	if !got.Active.Stale {
		t.Error("Active.Stale = false, want true")
	}
	if !got.Active.DifferentCLIVersion {
		t.Error("Active.DifferentCLIVersion = false, want true")
	}
}

func TestDoctor_ActiveFreshAndSameVersion(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	handofftest.FixedClock(t, now)

	active := &handoff.Active{
		Key: "PROJ-1", Stage: "implement", RepoRoots: []string{}, ToolConfigFiles: []string{},
		AllowlistHash: exec.Hash(), LastEventAt: now.Add(-1 * time.Minute).Format(time.RFC3339),
	}
	if _, err := handoff.SaveActive(home, active); err != nil {
		t.Fatalf("SaveActive: %v", err)
	}

	got := runDoctor(t, home)
	if got.Active == nil {
		t.Fatal("Active = nil, want a report")
	}
	if got.Active.Stale {
		t.Error("Active.Stale = true, want false")
	}
	if got.Active.DifferentCLIVersion {
		t.Error("Active.DifferentCLIVersion = true, want false")
	}
}

func TestDoctor_LastSessionHost(t *testing.T) {
	home := t.TempDir()

	got := runDoctor(t, home)
	if got.LastSessionHost != nil {
		t.Errorf("LastSessionHost = %v, want nil (no session_start writer exists before phase 9)", *got.LastSessionHost)
	}

	// Hand-write a session_start-shaped line directly, rather than
	// going through `hook session-start` itself (see hook_test.go's own
	// TestHookSessionStart_CreatesConfigAndAppendsEvent for that path) —
	// this test only needs to prove the tolerant reader recognizes the
	// shape.
	line := []byte(`{"at":"2026-01-01T00:00:00Z","type":"session_start","host":"claude-code"}` + "\n")
	if err := os.WriteFile(handoff.GlobalEventsPath(home), line, 0o600); err != nil {
		t.Fatalf("write events.jsonl: %v", err)
	}

	got = runDoctor(t, home)
	if got.LastSessionHost == nil || *got.LastSessionHost != "claude-code" {
		t.Errorf("LastSessionHost = %v, want \"claude-code\"", got.LastSessionHost)
	}
}

func TestDoctor_HostsNotDetected(t *testing.T) {
	home := t.TempDir()
	got := runDoctor(t, home)
	if len(got.Hosts) != 2 {
		t.Fatalf("Hosts = %+v, want 2 entries", got.Hosts)
	}
	for _, h := range got.Hosts {
		if h.Detected {
			t.Errorf("host %q: Detected = true in a bare temp home, want false", h.Host)
		}
	}
}

func TestDoctor_HostsDetected(t *testing.T) {
	home := t.TempDir()
	manifestDir := filepath.Join(home, "plugins", "ai-flow", ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(`{"version":"0.4.0"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := runDoctor(t, home)
	found := false
	for _, h := range got.Hosts {
		if h.Host == "claude-code" {
			found = true
			if !h.Detected || h.Version != "0.4.0" {
				t.Errorf("claude-code host report = %+v, want detected 0.4.0", h)
			}
		}
	}
	if !found {
		t.Fatalf("Hosts = %+v, missing claude-code", got.Hosts)
	}
}

func TestDoctor_HookSelftest(t *testing.T) {
	home := t.TempDir()
	got := runDoctor(t, home)
	if len(got.HookSelftest) != 2 {
		t.Fatalf("HookSelftest = %+v, want 2 entries", got.HookSelftest)
	}
	for _, r := range got.HookSelftest {
		if !r.Passed {
			t.Errorf("host %q: Passed = false, want true (embedded fixtures should always self-test clean)", r.Host)
		}
	}
}
