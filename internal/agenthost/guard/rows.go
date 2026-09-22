package guard

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// RowInfo is one protected-table row's documentation metadata —
// carried for tests and for doctor/future introspection only. It
// never drives Evaluate's own matching logic (see the plan's design
// decision 2): each row below is its own small, commented check.
type RowInfo struct {
	ID            string
	Kind          string // "path" | "command"
	Scope         string
	When          string
	OwningCommand string
}

// Rows documents the Guardrails section's own seven-row table, in its
// own order.
func Rows() []RowInfo {
	return []RowInfo{
		{ID: "row1_ticket_dir", Kind: "path", Scope: "the path itself", When: "that ticket is active",
			OwningCommand: "stage complete / ops … / mutation … / test … / visual … / e2e … / summary render"},
		{ID: "row2_standards_config_active", Kind: "path", Scope: "the path itself", When: "a ticket is active",
			OwningCommand: "standards sync; edit configuration between tickets"},
		{ID: "row3_cache", Kind: "path", Scope: "the path itself", When: "always",
			OwningCommand: "ops render / ops drift / ops dry-run"},
		{ID: "row4_waivers", Kind: "path", Scope: "inside an active ticket's in-scope repository", When: "always",
			OwningCommand: "standards waive"},
		{ID: "row5_tool_config", Kind: "path", Scope: "inside that ticket's in-scope repository", When: "that ticket is active",
			OwningCommand: "fix at the source"},
		{ID: "row6_git_publish", Kind: "command", Scope: "cwd inside an active ticket's in-scope repository", When: "that ticket is active",
			OwningCommand: "git commit / git push / pr create"},
		{ID: "row7_infra_test_mutation_tools", Kind: "command", Scope: "cwd inside an active ticket's in-scope repository", When: "that ticket is active",
			OwningCommand: "the equivalent ai-flow ops … / standards check / test run / mutation run"},
	}
}

// proseArtifacts is row 1's exclusion list — the Handoff directory
// table's own "agent"-written files, which a skill writes directly and
// the guard must never block.
var proseArtifacts = []string{
	"plan.md", "implementation.log", "impact.md", "rollback-plan.md",
	"review.md", "commit-message.md", "pr-body.md",
}

// evaluateWrite applies rows 1-5 (every path-kind row) to a Write
// event, in table order.
func evaluateWrite(ev agenthost.HookEvent, active *handoff.Active, home string) agenthost.Decision {
	if ev.FilePath == "" {
		// "A write payload without a recognisable path is allowed and
		// logged as hook_shape_unknown" — Guardrails' own words, verbatim.
		return allowDecision("hook_shape_unknown")
	}

	// Row 3: ~/.ai-flow/cache/** — always, no active ticket required.
	if isWithin(ev.FilePath, filepath.Join(home, "cache")) {
		return denyDecision("row3_cache", "ops render / ops drift / ops dry-run",
			"the real rendering cache is never an artifact")
	}

	// Rows 2, 1, 4 and 5 all require a ticket to be active — a single
	// guard around all four, in the protected table's own row order,
	// rather than repeating "if active != nil" per row.
	if active != nil {
		// Row 2: standards/, config.json, active.json — only while a
		// ticket (any ticket) is active.
		for _, p := range []string{filepath.Join(home, "standards"), filepath.Join(home, "config.json"), filepath.Join(home, "active.json")} {
			if isWithin(ev.FilePath, p) || ev.FilePath == p {
				return denyDecision("row2_standards_config_active", "standards sync",
					"standards/config/active.json are protected while a ticket is active")
			}
		}

		// Row 1: the active ticket's own handoff directory, except the
		// prose artifacts a skill writes directly.
		ticketDir := handoff.TicketDir(home, active.Key)
		if isWithin(ev.FilePath, ticketDir) {
			rel, err := filepath.Rel(ticketDir, ev.FilePath)
			if err == nil && !slices.Contains(proseArtifacts, rel) {
				return denyDecision("row1_ticket_dir", "stage complete / summary render / the equivalent ai-flow command",
					"only ai-flow writes machine-readable handoff files")
			}
		}

		// Row 4: <repo>/.ai-flow/waivers.json, inside an active ticket's
		// in-scope repository.
		for _, root := range active.RepoRoots {
			if isWithin(ev.FilePath, root) {
				rel, err := filepath.Rel(root, ev.FilePath)
				if err == nil && rel == filepath.Join(".ai-flow", "waivers.json") {
					return denyDecision("row4_waivers", "standards waive", "waivers are recorded by ai-flow, with a reason and an expiry")
				}
			}
		}

		// Row 5: tool configuration files named in active.json, inside
		// that ticket's in-scope repository. ToolConfigFiles is empty
		// until phase 14 populates it, so this never fires today — a
		// forward-compatible stub, same shape as phase 8's
		// doctorLastSessionHost.
		if slices.Contains(active.ToolConfigFiles, ev.FilePath) {
			return denyDecision("row5_tool_config", "fix at the source", "tool configuration is resolved by ai-flow's standards packs")
		}
	}

	return allowDecision("allow")
}

// row6Denied is the fixed (tool, verb) deny-list row 6 always blocks —
// checked ahead of, and independently from, row 7's exec.Table reuse:
// git commit/push both also match exec.Table (the CLI's own rows for
// running them after a recorded ship approval), but a developer typing
// them directly must never be treated the same as the CLI's own call.
var row6Denied = map[string][][]string{
	"git": {{"commit"}, {"push"}, {"tag"}},
	"gh":  {{"pr"}},
}

// row7Tools is the fixed infra-tool name set exec.Table carries no
// rows for yet (no addon exists before phase 18) — every invocation is
// denied, since a miss against exec.Table is what row 7 denies on.
var row7InfraTools = []string{"terraform", "kubectl", "helm", "kustomize", "act", "jenkins-cli", "kind"}

// execAllowlist wraps exec.Table once, at package init, rather than
// inside evaluateShell — exec.Table never changes at runtime (it is a
// fixed lookup table, like every other var in this file), so rebuilding
// an Allowlist from it on every single hook guard invocation would be
// pure waste.
var execAllowlist = exec.NewAllowlist(exec.Table...)

// evaluateShell applies rows 6 and 7 to a Shell/Bash event, in table
// order.
func evaluateShell(ev agenthost.HookEvent, active *handoff.Active) agenthost.Decision {
	if active == nil {
		return allowDecision("allow")
	}
	argv := splitCommand(ev.Command)
	if len(argv) == 0 {
		return allowDecision("hook_shape_unknown")
	}
	if !cwdInScope(ev.Cwd, active.RepoRoots) {
		return allowDecision("allow_out_of_scope")
	}

	tool := argv[0]

	// Row 6: git publish verbs and gh pr — always denied in scope,
	// regardless of exec.Table.
	if verbs, ok := row6Denied[tool]; ok {
		for _, verb := range verbs {
			if verbMatches(argv[1:], verb) {
				return denyDecision("row6_git_publish", "git commit / git push / pr create",
					"publishing requires a recorded ship approval, which only ai-flow checks")
			}
		}
	}

	// Row 7: infra tools with no addon yet (always denied in scope), and
	// every check/test/mutation tool exec.Table already names — allowed
	// only when the exact invocation matches one of its rows (see the
	// plan's design decision 4 for the accepted pytest-wildcard gap).
	if slices.Contains(row7InfraTools, tool) {
		return denyDecision("row7_infra_test_mutation_tools", "the equivalent ai-flow ops … command",
			"infrastructure tools are run by ai-flow, never directly")
	}
	if isRow7TestMutationTool(tool) {
		if _, ok := execAllowlist.Match(argv); ok {
			return allowDecision("allow")
		}
		return denyDecision("row7_infra_test_mutation_tools", "test run / mutation run",
			"test and mutation runners are invoked by ai-flow, so results land in the handoff directory")
	}

	return allowDecision("allow")
}

// isRow7TestMutationTool reports whether tool is one of exec.Table's
// own check/test/mutation tool names — derived from the table itself
// rather than hardcoded a second time, so a future phase's new
// mutation tool package is automatically guarded the moment its row
// lands in exec.Table.
func isRow7TestMutationTool(tool string) bool {
	for _, row := range exec.Table {
		if row.Tool == tool && row.Class == exec.ClassNone {
			return true
		}
	}
	return false
}

// isWithin reports whether path is root itself or lies inside it,
// comparing cleaned paths so "/repo-other" never matches root "/repo".
func isWithin(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func cwdInScope(cwd string, roots []string) bool {
	for _, root := range roots {
		if isWithin(cwd, root) {
			return true
		}
	}
	return false
}

// splitCommand does the smallest possible shell-word split this
// package needs: whitespace-separated tokens, no quoting support. Real
// shell commands the guard denies are simple tool invocations
// (`git push`, `kubectl apply ...`); a quoted argument only ever
// appears after the verb position this package matches on, so a naive
// split never changes tool/verb matching.
func splitCommand(command string) []string {
	return strings.Fields(command)
}

func verbMatches(rest, verb []string) bool {
	if len(rest) < len(verb) {
		return false
	}
	return slices.Equal(rest[:len(verb)], verb)
}
