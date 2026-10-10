package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// TestStageStartRecordsToolConfigFiles is SEC-2 end to end: after
// `stage start`, active.json names the repository's lint and mutation
// tool configuration (go.lint → golangci-lint, gremlins detected from
// go.mod), and `hook guard` denies an edit to one of them — given as a
// relative path from the repository, which only works because the guard
// normalizes paths (3.1) before row 5's exact comparison.
func TestStageStartRecordsToolConfigFiles(t *testing.T) {
	home, repoDir := standardsTicket(t, "PROJ-7")
	writeFileTest(t, filepath.Join(repoDir, ".golangci.yml"), "version: \"2\"\n")
	writeFileTest(t, filepath.Join(repoDir, ".gremlins.yaml"), "unleash: {}\n")
	writeFileTest(t, filepath.Join(repoDir, ".eslintrc.json"), "{}\n") // no eslint check applies to a Go repo

	if stdout, _, ec := runRoot(t, "stage", "start", "PROJ-7", "plan", "--home", home); ec != 0 {
		t.Fatalf("stage start: exitCode = %d\n%s", ec, stdout.String())
	}
	a, err := handoff.LoadActive(home)
	if err != nil || a == nil {
		t.Fatalf("LoadActive = %v, %v", a, err)
	}
	want := []string{filepath.Join(repoDir, ".golangci.yml"), filepath.Join(repoDir, ".gremlins.yaml")}
	slices.Sort(want)
	if !slices.Equal(a.ToolConfigFiles, want) {
		t.Fatalf("tool_config_files = %v, want %v", a.ToolConfigFiles, want)
	}

	t.Setenv("CLAUDE_PLUGIN_ROOT", "/plugin")
	// An allow prints no decision at all (normal permission flow), so
	// it reads back as "".
	for path, wantDecision := range map[string]string{".golangci.yml": "deny", "main.go": ""} {
		payload, _ := json.Marshal(map[string]any{
			"hook_event_name": "PreToolUse", "cwd": repoDir, "tool_name": "Write",
			"tool_input": map[string]any{"file_path": path, "content": "x"},
		})
		stdout, _, ec := runRootStdin(t, string(payload), "hook", "guard", "--home", home)
		var resp struct {
			HookSpecificOutput struct {
				PermissionDecision string `json:"permissionDecision"`
			} `json:"hookSpecificOutput"`
		}
		if ec != 0 || (stdout.Len() > 0 && json.Unmarshal(stdout.Bytes(), &resp) != nil) {
			t.Fatalf("hook guard %s: exitCode = %d\n%s", path, ec, stdout.String())
		}
		if got := resp.HookSpecificOutput.PermissionDecision; got != wantDecision {
			t.Errorf("write %s: permissionDecision = %q, want %q", path, got, wantDecision)
		}
	}
}

func TestToolConfigFilesBeforeRepoJSONIsEmpty(t *testing.T) {
	got, err := toolConfigFilesFor(t.TempDir(), "PROJ-1")
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("toolConfigFilesFor with no repo.json = %v, %v; want [], nil", got, err)
	}
}
