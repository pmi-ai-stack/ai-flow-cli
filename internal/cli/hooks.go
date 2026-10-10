package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// hooksInstallScope is --scope's two values.
type hooksInstallScope string

const (
	scopeProject hooksInstallScope = "project"
	scopeUser    hooksInstallScope = "user"
)

func newHooksCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Install hook bindings for a host with no plugin installed",
	}
	cmd.AddCommand(newHooksInstallCmd(cio))
	return cmd
}

type hooksInstallResult struct {
	Host   string `json:"host"`
	Scope  string `json:"scope"`
	Path   string `json:"path"`
	Merged bool   `json:"merged"`
}

// newHooksInstallCmd builds `hooks install --host H --scope
// project|user` — see the plan's design decision 7 for the exact
// write targets and command strings. This is the no-plugin path the
// Hosts table's own "No-plugin" row names for skills
// (ai-flow skills install --into .claude/skills); this command is its
// hooks equivalent.
func newHooksInstallCmd(cio cmdIO) *cobra.Command {
	var hostFlag, scopeFlag string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write hook bindings directly, for a checkout with no plugin installed",
		Args:  cobra.NoArgs,
		RunE: wrap("hooks install", cio.stdout, cio.exitCode, func(_ *cobra.Command, _ []string) (Outcome, error) {
			_, host, err := adapterForHostFlag(hostFlag)
			if err != nil {
				return Outcome{}, err
			}
			scope, err := parseScope(scopeFlag)
			if err != nil {
				return Outcome{}, err
			}

			path, merged, err := installHooks(host, scope)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: hooksInstallResult{Host: string(host), Scope: string(scope), Path: path, Merged: merged}}, nil
		}),
	}
	cmd.Flags().StringVar(&hostFlag, "host", "", "claude-code or cursor")
	cmd.Flags().StringVar(&scopeFlag, "scope", "", "project or user")
	return cmd
}

func parseScope(s string) (hooksInstallScope, error) {
	switch hooksInstallScope(s) {
	case scopeProject, scopeUser:
		return hooksInstallScope(s), nil
	default:
		return "", &envelope.Error{Code: "usage", Message: fmt.Sprintf("--scope must be %q or %q, got %q", scopeProject, scopeUser, s)}
	}
}

// scopeRoot resolves scope to the base directory the written
// --plugin-root points at: cwd for project, the user's home directory
// for user — there is no real plugin directory in this no-plugin
// path, so the base directory is the closest stand-in (the plan's
// design decision 7).
func scopeRoot(scope hooksInstallScope) (string, error) {
	if scope == scopeProject {
		return os.Getwd()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cli: resolve user home directory: %w", err)
	}
	return home, nil
}

// installHooks writes host's hook bindings at scope and returns the
// path written and whether it was a merge (Claude Code, into
// .claude/settings.json's own "hooks" key) or a full-file write
// (Cursor, .cursor/hooks.json — this file has no other purpose).
func installHooks(host agenthost.Host, scope hooksInstallScope) (path string, merged bool, err error) {
	root, err := scopeRoot(scope)
	if err != nil {
		return "", false, err
	}

	switch host {
	case agenthost.HostClaudeCode:
		path = filepath.Join(root, ".claude", "settings.json")
		if err := mergeClaudeCodeHooks(path, root); err != nil {
			return "", false, err
		}
		return path, true, nil
	case agenthost.HostCursor:
		path = filepath.Join(root, ".cursor", "hooks.json")
		if err := writeCursorHooks(path, root); err != nil {
			return "", false, err
		}
		return path, false, nil
	default:
		return "", false, fmt.Errorf("cli: unknown host %q", host)
	}
}

func sessionStartCommand(pluginRoot string) string {
	return fmt.Sprintf("ai-flow hook session-start --plugin-root %s", pluginRoot)
}

const guardCommand = "ai-flow hook guard"

// claudeCodeHooksBlock is the "hooks" key's own shape, matching
// ai-flow/hooks/claude-code.json's structure exactly, except each
// command calls the CLI directly — there is no guard.sh/
// session-start.sh to point at in this no-plugin path.
type claudeCodeHooksBlock struct {
	SessionStart []claudeCodeHookGroup `json:"SessionStart"`
	PreToolUse   []claudeCodeHookGroup `json:"PreToolUse"`
}

type claudeCodeHookGroup struct {
	Matcher string              `json:"matcher,omitempty"`
	Hooks   []claudeCodeHookCmd `json:"hooks"`
}

type claudeCodeHookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func claudeCodeHooks(pluginRoot string) claudeCodeHooksBlock {
	return claudeCodeHooksBlock{
		SessionStart: []claudeCodeHookGroup{
			{Hooks: []claudeCodeHookCmd{{Type: "command", Command: sessionStartCommand(pluginRoot)}}},
		},
		PreToolUse: []claudeCodeHookGroup{
			{Matcher: "Bash", Hooks: []claudeCodeHookCmd{{Type: "command", Command: guardCommand}}},
			{Matcher: "Write|Edit|MultiEdit|NotebookEdit", Hooks: []claudeCodeHookCmd{{Type: "command", Command: guardCommand}}},
		},
	}
}

// mergeClaudeCodeHooks reads path (a Claude Code settings.json, read
// by both hosts per the Guardrails section), replaces only its
// top-level "hooks" key and writes it back — every other key the file
// already carries survives untouched. A missing file starts from an
// empty object.
func mergeClaudeCodeHooks(path, pluginRoot string) error {
	settings := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil { //nolint:gosec // G304: path is built from --scope/cwd/home, not attacker input
		if err := json.Unmarshal(data, &settings); err != nil {
			return &envelope.Error{Code: "usage", Message: fmt.Sprintf("%s does not contain a JSON object: %v", path, err)}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cli: read %s: %w", path, err)
	}

	hooksData, err := marshalIndent(claudeCodeHooks(pluginRoot))
	if err != nil {
		return err
	}
	settings["hooks"] = hooksData

	data, err := marshalIndent(settings)
	if err != nil {
		return err
	}
	return atomicWriteHooksFile(path, data)
}

// cursorHooksFile mirrors ai-flow/hooks/cursor.json's structure
// exactly, commands pointed at the CLI directly (see
// claudeCodeHooksBlock's own doc comment).
type cursorHooksFile struct {
	Version int         `json:"version"`
	Hooks   cursorHooks `json:"hooks"`
}

type cursorHooks struct {
	SessionStart []cursorSessionStartHook `json:"sessionStart"`
	PreToolUse   []cursorPreToolUseHook   `json:"preToolUse"`
}

type cursorSessionStartHook struct {
	Command string `json:"command"`
}

type cursorPreToolUseHook struct {
	Matcher    string `json:"matcher"`
	FailClosed bool   `json:"failClosed,omitempty"`
	Command    string `json:"command"`
}

func writeCursorHooks(path, pluginRoot string) error {
	file := cursorHooksFile{
		Version: 1,
		Hooks: cursorHooks{
			SessionStart: []cursorSessionStartHook{{Command: sessionStartCommand(pluginRoot)}},
			PreToolUse: []cursorPreToolUseHook{
				{Matcher: "Shell", FailClosed: true, Command: guardCommand},
				{Matcher: "Write|Delete", Command: guardCommand},
			},
		},
	}
	data, err := marshalIndent(file)
	if err != nil {
		return err
	}
	return atomicWriteHooksFile(path, data)
}

// marshalIndent is this file's own copy of 07-determinism.md's write
// shape (two-space indent, HTML escaping disabled, trailing newline) —
// internal/handoff's marshalCanonical is unexported and these files
// are not handoff artifacts (they live inside a git-tracked
// repository, under a host's own config directory, not under
// ~/.ai-flow/).
func marshalIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("cli: marshal: %w", err)
	}
	return buf.Bytes(), nil
}

// atomicWriteHooksFile writes data to path as a temp file in the same
// directory, synced and closed, then renamed into place — the same
// crash-safety shape every other writer in this codebase uses.
func atomicWriteHooksFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cli: create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".hooks.json.tmp-*")
	if err != nil {
		return fmt.Errorf("cli: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cli: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cli: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cli: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("cli: rename into place: %w", err)
	}
	cleanup = false
	return nil
}
