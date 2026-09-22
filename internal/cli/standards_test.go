package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

func TestStandardsValidate_ReferencePack(t *testing.T) {
	stdout, _, exitCode := runRoot(t, "standards", "validate", "../../packs/reference")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result struct {
			Valid    bool `json:"valid"`
			Sections int  `json:"sections"`
			Checks   int  `json:"checks"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if !env.Result.Valid {
		t.Error("result.valid = false, want true")
	}
	if env.Result.Sections == 0 {
		t.Error("result.sections = 0, want > 0")
	}
}

func TestStandardsValidate_InvalidExitsSix(t *testing.T) {
	dir := t.TempDir()
	writeFileTest(t, filepath.Join(dir, "standards.json"), `{
  "schema_version": 1, "id": "bad", "version": "1.0.0",
  "sections": [],
  "checks": [{ "id": "x", "kind": "scan", "tool": "totally-unknown-tool", "severity": "must" }]
}`)

	stdout, _, exitCode := runRoot(t, "standards", "validate", dir)
	if exitCode != 6 {
		t.Fatalf("exitCode = %d, want 6\n%s", exitCode, stdout.String())
	}
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string   `json:"code"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.OK {
		t.Error("ok = true, want false")
	}
	if env.Error.Code != "check_runner_missing" {
		t.Errorf("error.code = %q, want %q", env.Error.Code, "check_runner_missing")
	}
	if len(env.Error.Details) == 0 {
		t.Error("error.details is empty")
	}
}

func TestStandardsExport_ThenValidateRoundTrips(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "forked-reference")

	stdout, _, exitCode := runRoot(t, "standards", "export", dest)
	if exitCode != 0 {
		t.Fatalf("export: exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	stdout, _, exitCode = runRoot(t, "standards", "validate", dest)
	if exitCode != 0 {
		t.Fatalf("validate exported dir: exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
}

func TestStandardsExport_RefusesNonEmptyDestination(t *testing.T) {
	dest := t.TempDir()
	writeFileTest(t, filepath.Join(dest, "already-here.txt"), "hi")

	stdout, _, exitCode := runRoot(t, "standards", "export", dest)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", exitCode, stdout.String())
	}
}

func TestStandardsSync_LocalSourceAndSimulate(t *testing.T) {
	home := t.TempDir()
	writeValidDoctorConfig(t, home) // empty standards.sources — overwritten below with a local org source

	packDir := t.TempDir()
	writeFileTest(t, filepath.Join(packDir, "standards.json"), `{"schema_version":1,"id":"local-pack","version":"1.0.0","sections":[],"checks":[]}`)
	cfg := `{
  "schema_version": 1,
  "tracker": { "provider": "jira", "base_url": "https://example.atlassian.net", "statuses": { "todo": ["To Do"], "in_progress": "In Progress", "in_review": "In Review" }, "bug_types": ["Bug"], "comment_limit": 5 },
  "host": { "provider": "github", "default_base_branch": "main", "draft_prs": false },
  "routing": { "project_key_map": {}, "keyword_to_repo": {}, "teams": {} },
  "defaults": { "worktree": false, "viewport": "1440x900", "mutation_threshold": 80, "mutation_max_rounds": 3, "mutation_timeout_minutes": 20, "test_timeout_minutes": 20, "guard_idle_hours": 8 },
  "standards": { "sources": [{ "id": "local-pack", "repo": "` + jsonEscape(packDir) + `", "ref": "local" }], "guidance_budget_tokens": 6000 },
  "ops": { "environments": { "dev": { "tier": "nonprod" } }, "target_map": {}, "default_environment": "dev", "risk_gate": 40, "risk": { "action_weight": {}, "kind_weight": {}, "tier_multiplier": {}, "drift_penalty": 5, "offline_penalty": 10 }, "protected_kinds": [], "e2e": { "enabled": false, "sandbox_environment": "" }, "tool_timeout_minutes": 30 }
}`
	writeFileTest(t, filepath.Join(home, "config.json"), cfg)

	// --simulate touches neither filesystem nor network.
	stdout, _, exitCode := runRoot(t, "standards", "sync", "--home", home, "--simulate")
	if exitCode != 0 {
		t.Fatalf("simulate sync: exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	if _, err := os.Stat(standards.CacheDir(home, "local-pack", "local")); err == nil {
		t.Fatal("--simulate synced for real")
	}

	stdout, _, exitCode = runRoot(t, "standards", "sync", "--home", home)
	if exitCode != 0 {
		t.Fatalf("sync: exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result struct {
			Synced []struct {
				ID      string `json:"id"`
				RefKind string `json:"ref_kind"`
				Path    string `json:"path"`
			} `json:"synced"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Synced) != 1 || env.Result.Synced[0].ID != "local-pack" {
		t.Fatalf("synced = %+v, want one local-pack entry", env.Result.Synced)
	}
	if _, err := os.Stat(filepath.Join(env.Result.Synced[0].Path, "standards.json")); err != nil {
		t.Errorf("synced pack content missing: %v", err)
	}
}

func writeFileTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b[1 : len(b)-1])
}
