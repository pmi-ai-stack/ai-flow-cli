package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
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
