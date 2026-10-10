package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// escapingTargets overwrites key's ops-targets.json with one good
// target and two whose repo or path escape their roots, as a tampered
// or mis-detected artifact would.
func escapingTargets(t *testing.T, home, key string) {
	t.Helper()
	targets := ops.DetectResult{
		AppFiles: []string{},
		Targets: []ops.Target{
			{Addon: "no-such-addon", Repo: "acme.infra", Path: "infra/prod", Scope: "repo", Files: []string{}},
			{Addon: "no-such-addon", Repo: "acme.infra", Path: "../../outside", Scope: "repo", Files: []string{}},
			{Addon: "no-such-addon", Repo: "..", Path: "infra", Scope: "repo", Files: []string{}},
		},
	}
	if _, err := handoff.WriteArtifact(filepath.Join(home, key), "ops-targets.json", "ops-targets", targets); err != nil {
		t.Fatalf("write ops-targets.json: %v", err)
	}
}

// TestOpsTargetsEscapingTargetIsInvalidResult: every reader of
// ops-targets.json refuses a target that would escape the ticket's
// render directory or the repository checkout — exit 6 invalid_result,
// one details line per bad target (security review 2026-09-27 S10).
func TestOpsTargetsEscapingTargetIsInvalidResult(t *testing.T) {
	home, _ := terraformTargetTicket(t, "PROJ-1")
	escapingTargets(t, home, "PROJ-1")

	stdout, _, ec := runRoot(t, "--home", home, "ops", "render", "PROJ-1", "--ref", "branch")
	if ec != 6 {
		t.Fatalf("exitCode = %d, want 6\n%s", ec, stdout.String())
	}
	var env struct {
		Error struct {
			Code    string   `json:"code"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "invalid_result" {
		t.Errorf("code = %q, want invalid_result", env.Error.Code)
	}
	want := []string{
		"acme.infra:../../outside: path must be relative and stay inside the repository",
		"..:infra: repo must be a name, not a path",
	}
	if !slices.Equal(env.Error.Details, want) {
		t.Errorf("details = %q\nwant      %q", env.Error.Details, want)
	}

	// The tolerant reader (impact, infra-test, checks, resolve) refuses
	// the same artifact the same way.
	_, err := loadOpsTargetsTolerant(filepath.Join(home, "PROJ-1"))
	var e *envelope.Error
	if !errors.As(err, &e) || e.Code != "invalid_result" {
		t.Errorf("loadOpsTargetsTolerant err = %v, want invalid_result", err)
	}
}
