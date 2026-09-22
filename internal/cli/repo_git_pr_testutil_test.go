package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// writeRoutingTestConfig writes a config.json whose routing section
// routes project key PROJ to Team Beta's two repositories, both
// cloned under localBase, and whose ops.tool_timeout_minutes/
// defaults.worktree are set from the given parameters — the shared
// fixture repo_test.go, git_test.go and pr_test.go all build on.
func writeRoutingTestConfig(t *testing.T, home, localBase string, worktree bool) {
	t.Helper()
	cfg := fmt.Sprintf(`{
  "schema_version": 1,
  "tracker": { "provider": "jira", "base_url": "http://127.0.0.1:0", "statuses": { "todo": ["To Do"], "in_progress": "In Progress", "in_review": "In Review" }, "bug_types": ["Bug"], "comment_limit": 0 },
  "host": { "provider": "github", "default_base_branch": "main", "draft_prs": false },
  "routing": {
    "project_key_map": { "PROJ": { "team": "Team Beta", "repos": ["acme.billing.api", "acme.billing.web"] } },
    "keyword_to_repo": { "billing": ["acme.billing.api", "acme.billing.web"] },
    "teams": { "Team Beta": { "local_base": %q, "repo_settings": {} } }
  },
  "defaults": { "worktree": %v, "viewport": "1440x900", "mutation_threshold": 80, "mutation_max_rounds": 3, "mutation_timeout_minutes": 20, "test_timeout_minutes": 20, "guard_idle_hours": 8 },
  "standards": { "sources": [], "guidance_budget_tokens": 6000 },
  "ops": { "environments": {}, "target_map": {}, "default_environment": "", "risk_gate": 40, "risk": { "action_weight": {}, "kind_weight": {}, "tier_multiplier": {}, "drift_penalty": 5, "offline_penalty": 10 }, "protected_kinds": [], "e2e": { "enabled": false, "sandbox_environment": "" }, "tool_timeout_minutes": 1 }
}`, localBase, worktree)
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// writeTicketFixture writes ticketDir/ticket.json directly (bypassing
// `ticket fetch`, which needs a real tracker) — every repo/git/pr test
// starts from a ticket already fetched.
func writeTicketFixture(t *testing.T, ticketDir, key, summary, description, branchName string) {
	t.Helper()
	if err := os.MkdirAll(ticketDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data := fmt.Sprintf(`{"key":%q,"summary":%q,"description":%q,"status":"In Progress","branch_name":%q}`,
		key, summary, description, branchName)
	if err := os.WriteFile(filepath.Join(ticketDir, "ticket.json"), []byte(data), 0o600); err != nil {
		t.Fatalf("write ticket.json: %v", err)
	}
}

// writeRepoFixture writes ticketDir/repo.json directly for tests that
// start after `repo locate` already ran.
func writeRepoFixture(t *testing.T, ticketDir, team string, repos []repoArtifactRepository) {
	t.Helper()
	if repos == nil {
		repos = []repoArtifactRepository{}
	}
	artifact := repoArtifact{Team: team, Repositories: repos}
	if _, err := handoff.WriteArtifact(ticketDir, "repo.json", "repo", artifact); err != nil {
		t.Fatalf("write repo.json: %v", err)
	}
}

// repoInstructionsDir creates localBase/name with a minimal AGENTS.md
// — the fixture every routing-dependent test needs so
// instructionsFileFor never refuses with missing_repo_instructions.
func repoInstructionsDir(t *testing.T, localBase, name string) string {
	t.Helper()
	dir := filepath.Join(localBase, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# rules\n"), 0o644); err != nil {
		t.Fatalf("WriteFile AGENTS.md: %v", err)
	}
	return dir
}

// newScriptedExecer is internal/cli's own tests' entry point into the
// shared exectest.Scripted double — kept as a thin wrapper so call
// sites read the same as before the /simplify pass that merged this
// with internal/gitops's independent copy.
func newScriptedExecer(t *testing.T) *exectest.Scripted {
	t.Helper()
	return exectest.New(t)
}

// useScriptedExecer swaps newExecer for the duration of the test,
// restoring the real constructor afterward (git.go/repo.go's
// buildGitOps calls newExecer(), a package var tests override — see
// git.go's own doc comment on newExecer).
func useScriptedExecer(t *testing.T, s *exectest.Scripted) {
	t.Helper()
	orig := newExecer
	newExecer = func() exec.Execer { return s }
	t.Cleanup(func() { newExecer = orig })
}
