package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/rollback"
)

func TestNonRevertibleSectionExtractsBoundedText(t *testing.T) {
	md := "# Rollback plan\n\n## Trigger criteria\n\nx\n\n## Non-revertible\n\naws_s3_bucket.data: snapshot taken before delete.\n\n## Watch list\n\ny\n"
	got := nonRevertibleSection(md)
	if got == "" {
		t.Fatal("section is empty")
	}
	if want := "aws_s3_bucket.data"; !strings.Contains(got, want) {
		t.Errorf("section = %q, want it to contain %q", got, want)
	}
	if strings.Contains(got, "Watch list") {
		t.Errorf("section = %q, must not bleed into the next heading", got)
	}
}

func TestNonRevertibleSectionMissingHeadingIsEmpty(t *testing.T) {
	if got := nonRevertibleSection("# Rollback plan\n\nno such section here\n"); got != "" {
		t.Errorf("section = %q, want empty", got)
	}
}

func TestCheckRollbackPlanCheckEmptyNonRevertibleNeverReadsFile(t *testing.T) {
	ticketDir := t.TempDir() // no rollback-plan.md written at all
	data, _ := json.Marshal(rollback.Result{NonRevertible: nil})
	if err := checkRollbackPlanCheck(ticketDir, data); err != nil {
		t.Errorf("err = %v, want nil (nothing to check)", err)
	}
}

func TestCheckRollbackPlanCheckMissingFileWithNonRevertibleFails(t *testing.T) {
	ticketDir := t.TempDir()
	data, _ := json.Marshal(rollback.Result{NonRevertible: []string{"aws_s3_bucket.data"}})
	err := checkRollbackPlanCheck(ticketDir, data)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "invalid_result" {
		t.Fatalf("err = %v, want *envelope.Error with Code invalid_result", err)
	}
}

func TestCheckRollbackPlanCheckCoveredAddressPasses(t *testing.T) {
	ticketDir := t.TempDir()
	md := "# Rollback plan\n\n## Non-revertible\n\n- aws_s3_bucket.data: snapshot taken.\n\n## Watch list\n\nnone\n"
	if err := os.WriteFile(filepath.Join(ticketDir, "rollback-plan.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(rollback.Result{NonRevertible: []string{"aws_s3_bucket.data"}})
	if err := checkRollbackPlanCheck(ticketDir, data); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestCheckRollbackPlanCheckPartiallyMissingNamesAddresses(t *testing.T) {
	ticketDir := t.TempDir()
	md := "# Rollback plan\n\n## Non-revertible\n\n- aws_s3_bucket.data: mitigated.\n"
	if err := os.WriteFile(filepath.Join(ticketDir, "rollback-plan.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(rollback.Result{NonRevertible: []string{"aws_s3_bucket.data", "aws_iam_role.x"}})
	err := checkRollbackPlanCheck(ticketDir, data)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "invalid_result" {
		t.Fatalf("err = %v, want *envelope.Error with Code invalid_result", err)
	}
	if len(envErr.Details) != 1 || envErr.Details[0] != "aws_iam_role.x" {
		t.Errorf("Details = %v, want [aws_iam_role.x]", envErr.Details)
	}
}

func TestOpsRollbackCommandWritesArtifact(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	ticketDir := filepath.Join(home, key)
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{{Name: "acme.infra", LocalPath: t.TempDir(), InstructionsFile: "AGENTS.md"}})

	tgt := ops.Target{Addon: "no-such-addon", Repo: "acme.infra", Path: "infra/prod", Scope: ops.ScopeDiff}
	writeOpsTargetsFixture(t, home, key, []ops.Target{tgt})
	writeDryRunResultsTargets(t, home, key, "live", []opsDryRunArtifactTarget{
		{Hash: "h1", Target: ops.TargetKey(tgt), Validation: []ops.Finding{}},
	})
	writePlanFixture(t, ticketDir, tgt, ops.Changeset{Actions: []ops.Action{
		{Address: "a", Addon: "no-such-addon", Tier: "nonprod", Kind: "Thing", KindClass: ops.KindOther, Action: ops.ActionUpdate, Target: ops.TargetKey(tgt)},
	}})

	stdout, _, ec := runRoot(t, "--home", home, "ops", "rollback", key)
	if ec != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", ec, stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "rollback-results.json"))
	if err != nil {
		t.Fatalf("rollback-results.json not written: %v", err)
	}
	var art struct {
		Repositories []struct {
			Repo string `json:"repo"`
		} `json:"repositories"`
		Targets []any `json:"targets"`
	}
	if err := json.Unmarshal(data, &art); err != nil {
		t.Fatal(err)
	}
	if len(art.Repositories) != 1 || art.Repositories[0].Repo != "acme.infra" {
		t.Errorf("Repositories = %+v, want one entry for acme.infra", art.Repositories)
	}
	// "no-such-addon" is never registered in buildOpsRegistry — the
	// same defensive-skip shape ops render/drift/dry-run already prove
	// against this exact fixture addon name.
	if len(art.Targets) != 0 {
		t.Errorf("Targets = %+v, want empty (unregistered addon skipped)", art.Targets)
	}
}

func TestOpsRollbackCatalogRegistration(t *testing.T) {
	found := false
	for _, c := range handoff.Catalog() {
		if c.Name == "ops rollback" {
			found = true
			if !c.TicketScoped {
				t.Error(`Catalog "ops rollback" TicketScoped = false, want true`)
			}
		}
	}
	if !found {
		t.Fatal(`Catalog() is missing "ops rollback"`)
	}
}
