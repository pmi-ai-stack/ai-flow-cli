package cli

import (
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// TestDoctor_EmptyStandardsSourcesReportsSyntheticReferenceRow matches
// config.Validate's own rule that an empty config.standards.sources
// means the embedded reference pack is the org layer — doctor's pack
// block must say the same thing config validate already does.
func TestDoctor_EmptyStandardsSourcesReportsSyntheticReferenceRow(t *testing.T) {
	home := t.TempDir()
	writeValidDoctorConfig(t, home) // standards.sources: []

	got := runDoctor(t, home)
	if len(got.Packs) != 1 {
		t.Fatalf("Packs = %+v, want exactly one synthetic row", got.Packs)
	}
	if got.Packs[0].ID != "reference" || got.Packs[0].Layer != "org" {
		t.Errorf("Packs[0] = %+v, want {ID: reference, Layer: org}", got.Packs[0])
	}
	if got.Packs[0].Valid == nil || !*got.Packs[0].Valid {
		t.Error("Packs[0].Valid should be true for the embedded reference pack")
	}
}

// TestDoctor_ConfiguredSourceReportsSyncStateAndValidity exercises a
// real (non-synthetic) row: unsynced before standards.Sync runs,
// synced and valid after.
func TestDoctor_ConfiguredSourceReportsSyncStateAndValidity(t *testing.T) {
	home := t.TempDir()
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

	before := runDoctor(t, home)
	if len(before.Packs) != 1 || before.Packs[0].Synced {
		t.Fatalf("before sync: Packs = %+v, want one unsynced row", before.Packs)
	}

	if _, err := standards.Sync(standards.SyncOptions{
		Home:    home,
		Sources: []standards.Source{{ID: "local-pack", Repo: packDir, Ref: "local", Layer: "org"}},
	}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	after := runDoctor(t, home)
	if len(after.Packs) != 1 {
		t.Fatalf("after sync: Packs = %+v, want one row", after.Packs)
	}
	if !after.Packs[0].Synced {
		t.Error("after sync: Synced = false, want true")
	}
	if after.Packs[0].Valid == nil || !*after.Packs[0].Valid {
		t.Error("after sync: Valid should be true for a manifest with no checks[]")
	}
}
