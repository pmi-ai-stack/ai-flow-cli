package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// runRootStdin is runRoot's stdin-carrying twin, for hook guard/hook
// session-start — the two commands that read a payload from stdin.
func runRootStdin(t *testing.T, stdin string, args ...string) (stdout, stderr bytes.Buffer, exitCode int) {
	t.Helper()
	exitCode = -1
	root := NewRoot(fixedBuildInfo, strings.NewReader(stdin), &stdout, &stderr, &exitCode)
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute(%v): %v", args, err)
	}
	return stdout, stderr, exitCode
}

func TestHookGuard_AllowsWhenNoActiveTicket(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/plugin")
	home := t.TempDir()
	payload := `{"hook_event_name":"PreToolUse","cwd":"/repo","tool_name":"Bash","tool_input":{"command":"git status"}}`

	stdout, _, exitCode := runRootStdin(t, payload, "hook", "guard", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var resp struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if resp.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %q, want allow", resp.HookSpecificOutput.PermissionDecision)
	}
}

func TestHookGuard_DeniesGitPushInActiveTicketRepo(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/plugin")
	home := t.TempDir()
	repo := filepath.Join(home, "..", "acme.billing.api")
	// A real ticket directory exists long before `ship` (created at
	// `ticket fetch`) — AppendEvent itself does not MkdirAll (writer.go's
	// atomic-write path does that; events.jsonl is append-only and
	// assumes the directory it belongs to already exists).
	if err := os.MkdirAll(filepath.Join(home, "PROJ-1"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeActiveFixture(t, home, "PROJ-1", "ship", []string{repo})

	payload := bashPushPayloadFor(repo)
	stdout, _, exitCode := runRootStdin(t, payload, "hook", "guard", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var resp struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if resp.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q, want deny", resp.HookSpecificOutput.PermissionDecision)
	}

	data, err := os.ReadFile(filepath.Join(home, "PROJ-1", "events.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile events.jsonl: %v", err)
	}
	if !strings.Contains(string(data), `"guard_decision"`) {
		t.Errorf("ticket events.jsonl missing guard_decision entry:\n%s", data)
	}
}

func bashPushPayloadFor(repo string) string {
	return `{"hook_event_name":"PreToolUse","cwd":"` + repo + `","tool_name":"Bash","tool_input":{"command":"git push origin feature/PROJ-1"}}`
}

func writeActiveFixture(t *testing.T, home, key, stage string, repoRoots []string) {
	t.Helper()
	a := &handoff.Active{Key: key, Stage: stage, RepoRoots: repoRoots, ToolConfigFiles: []string{}, LastEventAt: handoff.Now().Format("2006-01-02T15:04:05Z07:00")}
	if _, err := handoff.SaveActive(home, a); err != nil {
		t.Fatalf("SaveActive: %v", err)
	}
}

func TestHookGuard_MalformedPayloadExits2(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/plugin")
	home := t.TempDir()
	_, _, exitCode := runRootStdin(t, `{"not valid`, "hook", "guard", "--home", home)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
}

func TestHookGuard_NoHostEnvExits2(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	t.Setenv("CURSOR_PLUGIN_ROOT", "")
	home := t.TempDir()
	_, _, exitCode := runRootStdin(t, `{}`, "hook", "guard", "--home", home)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
}

func TestHookGuard_CursorShape(t *testing.T) {
	t.Setenv("CURSOR_PLUGIN_ROOT", "/plugin")
	home := t.TempDir()
	payload := `{"hook_event_name":"preToolUse","cwd":"/repo","tool":"Shell","input":{"command":"git status"}}`

	stdout, _, exitCode := runRootStdin(t, payload, "hook", "guard", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var resp struct {
		Permission string `json:"permission"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if resp.Permission != "allow" {
		t.Errorf("permission = %q, want allow", resp.Permission)
	}
}

func TestHookSessionStart_CreatesConfigAndAppendsEvent(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/plugin")
	home := t.TempDir()
	payload := `{"hook_event_name":"SessionStart","version":"2.1.0"}`

	stdout, _, exitCode := runRootStdin(t, payload, "hook", "session-start", "--plugin-root", "/plugin", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(home, "config.json")); err != nil {
		t.Errorf("config.json was not created: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile events.jsonl: %v", err)
	}
	var e handoff.Event
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &e); err != nil {
		t.Fatalf("decode last event: %v", err)
	}
	if e.Type != "session_start" || e.Host != "claude-code" || e.HostVersion != "2.1.0" || e.CLIVersion != fixedBuildInfo.Version {
		t.Errorf("event = %+v, want a matching session_start", e)
	}
}

func TestHookSessionStart_DoesNotOverwriteExistingConfig(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/plugin")
	home := t.TempDir()
	if _, _, exitCode := runRoot(t, "config", "init", "--home", home); exitCode != 0 {
		t.Fatalf("config init failed")
	}
	before, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if _, _, exitCode := runRootStdin(t, `{}`, "hook", "session-start", "--home", home); exitCode != 0 {
		t.Fatalf("hook session-start failed")
	}
	after, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("hook session-start modified an existing config.json")
	}
}

// writeConfigWithStandardsSources writes a config.json identical to
// config.Init's own template except for config.standards.sources —
// the init-then-load-then-patch-one-field-then-marshal shape both
// syncAbsentPacks tests below need, factored out once rather than
// repeated per test, matching this test file's own family's established
// per-file write<X>Config helper convention (see e.g. doctor_test.go's
// writeValidDoctorConfig).
func writeConfigWithStandardsSources(t *testing.T, home string, sources []config.StandardsSource) {
	t.Helper()
	if _, err := config.Init(home, ""); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	cfg, err := config.Load(home, "")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.Standards.Sources = sources
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// TestSyncAbsentPacksSyncsMissingLocalSource is phase 30's own test for
// the fix described in hook.go's own doc comment: syncAbsentPacks used
// to be a no-op, so no session-start ever actually synced a configured
// pack source. A local filesystem source needs no network, matching
// internal/standards's own TestSyncTagCommitAndLocalPathProduceIdenticalTrees
// precedent for exercising Sync without a real remote.
func TestSyncAbsentPacksSyncsMissingLocalSource(t *testing.T) {
	home := t.TempDir()

	localSrc := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localSrc, "org", "guidance"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema_version": 1, "id": "acme", "version": "1.0.0", "sections": [], "checks": []}`
	if err := os.WriteFile(filepath.Join(localSrc, "standards.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	writeConfigWithStandardsSources(t, home, []config.StandardsSource{{ID: "acme", Repo: localSrc, Ref: "local"}})

	syncAbsentPacks(context.Background(), home, "")

	cacheDir := standards.CacheDir(home, "acme", "local")
	if _, err := os.Stat(filepath.Join(cacheDir, "standards.json")); err != nil {
		t.Errorf("expected %s/standards.json to be synced: %v", cacheDir, err)
	}
}

// TestSyncAbsentPacksLeavesAnAlreadySyncedSourceAlone proves the
// "absent" half of the name: a source whose cache directory already
// exists is left untouched, never re-fetched, matching session-start's
// own "must stay fast on every turn but the first" requirement — a
// marker file inside the already-synced directory survives the call
// unmodified, which a real re-Sync (a fresh directory copy) would not
// guarantee.
func TestSyncAbsentPacksLeavesAnAlreadySyncedSourceAlone(t *testing.T) {
	home := t.TempDir()
	writeConfigWithStandardsSources(t, home, []config.StandardsSource{{ID: "acme", Repo: "https://example.invalid/never-reached.git", Ref: "v1.0.0"}})

	cacheDir := standards.CacheDir(home, "acme", "v1.0.0")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(cacheDir, "already-here")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	syncAbsentPacks(context.Background(), home, "") // must not attempt to reach example.invalid

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("already-synced source was touched: %v", err)
	}
}

// TestPruneStaleRenderCacheIfDue_RemovesOldEntriesOnceThenThrottles
// covers pruneStaleRenderCacheIfDue's own two jobs — applying
// render.DefaultCacheMaxAge, and not re-scanning within
// renderCachePruneThrottle of the last run.
func TestPruneStaleRenderCacheIfDue_RemovesOldEntriesOnceThenThrottles(t *testing.T) {
	home := t.TempDir()
	oldEntry := filepath.Join(home, "cache", "render", "old-hash")
	if err := os.MkdirAll(oldEntry, 0o700); err != nil {
		t.Fatal(err)
	}
	old := handoff.Now().Add(-render.DefaultCacheMaxAge - 24*time.Hour)
	if err := os.Chtimes(oldEntry, old, old); err != nil {
		t.Fatal(err)
	}

	pruneStaleRenderCacheIfDue(home)
	if _, err := os.Stat(oldEntry); !os.IsNotExist(err) {
		t.Fatalf("old cache entry survived the first prune: err=%v", err)
	}

	// Recreate the same stale entry and prune again immediately: the
	// throttle marker pruneStaleRenderCacheIfDue just wrote must skip
	// this second call entirely.
	if err := os.MkdirAll(oldEntry, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldEntry, old, old); err != nil {
		t.Fatal(err)
	}
	pruneStaleRenderCacheIfDue(home)
	if _, err := os.Stat(oldEntry); err != nil {
		t.Errorf("throttled prune should not have run again: %v", err)
	}
}

func TestHookSelftest_BothHosts(t *testing.T) {
	for _, host := range []string{"claude-code", "cursor"} {
		t.Run(host, func(t *testing.T) {
			stdout, _, exitCode := runRoot(t, "hook", "selftest", "--host", host)
			if exitCode != 0 {
				t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
			}
			var env struct {
				Result struct {
					Passed bool `json:"passed"`
				} `json:"result"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("stdout not valid JSON: %v", err)
			}
			if !env.Result.Passed {
				t.Errorf("passed = false\n%s", stdout.String())
			}
		})
	}
}

func TestHookSelftest_UnknownHost(t *testing.T) {
	_, _, exitCode := runRoot(t, "hook", "selftest", "--host", "vscode")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)", exitCode)
	}
}
