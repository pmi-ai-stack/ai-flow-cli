package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

func readSARIF(t *testing.T, path string) sarifDoc {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc sarifDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("SARIF is not JSON: %v\n%s", err, data)
	}
	return doc
}

// TestStandardsCheckDirSARIFOnFailure: a run that fails with open must
// findings still writes the SARIF file — that is when CI needs it —
// and stdout is the same envelope as without --sarif.
func TestStandardsCheckDirSARIFOnFailure(t *testing.T) {
	repo := stageTerraformE2E(t)
	useExecer(t, &toolFunc{answer: func(tool string, _ []string) (exec.Output, error) {
		switch tool {
		case "conftest":
			return exec.Output{Stdout: conftestTwoFailures, ExitCode: 1}, &exitOneToolFailed
		case "checkov":
			return exec.Output{Stdout: `{"results":{"failed_checks":[]}}`}, nil
		}
		return exec.Output{}, nil
	}})
	sarif := filepath.Join(t.TempDir(), "out", "ai-flow.sarif")

	env := runDirCheck(t, 6, "standards", "check", repo, "--home", t.TempDir(), "--sarif", sarif)
	if env.Error.Code != "invalid_result" || len(env.Error.Details) != 2 {
		t.Fatalf("error = %+v, want invalid_result with the two policy failures", env.Error)
	}

	doc := readSARIF(t, sarif)
	results := doc.Runs[0].Results
	if len(results) != 2 {
		t.Fatalf("%d results, want 2", len(results))
	}
	for _, r := range results {
		uri := r.Locations[0].PhysicalLocation.ArtifactLocation.URI
		if r.RuleID != "infra.terraform.policy" || r.Level != "error" || uri != "main.tf" {
			t.Errorf("result = %s %s %s, want infra.terraform.policy error main.tf", r.RuleID, r.Level, uri)
		}
	}
}

// TestStandardsCheckDirSARIFOnPass: a clean run writes an empty run, so
// code scanning can close alerts that were fixed.
func TestStandardsCheckDirSARIFOnPass(t *testing.T) {
	repo := stageTerraformE2E(t)
	useExecer(t, &toolFunc{answer: func(tool string, _ []string) (exec.Output, error) {
		switch tool {
		case "conftest":
			return exec.Output{Stdout: `[{"filename":"main.tf","namespace":"main","failures":[]}]`}, nil
		case "checkov":
			return exec.Output{Stdout: `{"results":{"failed_checks":[]}}`}, nil
		}
		return exec.Output{}, nil
	}})
	sarif := filepath.Join(t.TempDir(), "ai-flow.sarif")

	runDirCheck(t, 0, "standards", "check", repo, "--home", t.TempDir(), "--sarif", sarif)
	if doc := readSARIF(t, sarif); len(doc.Runs) != 1 || len(doc.Runs[0].Results) != 0 {
		t.Errorf("want one run with no results, got %+v", doc)
	}
}

// TestStandardsCheckSARIFWithTicketIsUsage: --sarif is the directory
// form's; a ticket's findings live in checks-results.json.
func TestStandardsCheckSARIFWithTicketIsUsage(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-9")
	env := runDirCheck(t, 2, "standards", "check", "PROJ-9", "--home", home, "--sarif", filepath.Join(t.TempDir(), "x.sarif"))
	if env.Error.Code != "usage" {
		t.Errorf("error = %+v, want usage", env.Error)
	}
}
