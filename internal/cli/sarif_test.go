package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff/handofftest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

// sarifFixtureFindings covers every shape the directory form produces:
// a repo-level finding with file and line, a target-scoped one whose
// file is relative to its target, one with no file at all, a waived one,
// a fixed one (omitted from SARIF), and the three severities.
func sarifFixtureFindings() []check.Finding {
	return []check.Finding{
		{CheckID: "go.lint", Layer: "org", Kind: "lint", Stage: "checks", Severity: "should", Tool: "golangci-lint",
			Repo: "acme", File: "cmd/main.go", Line: 12, Message: "unused variable x", Status: "open"},
		{CheckID: "infra.terraform.policy", Layer: "org", Kind: "policy", Stage: "checks", Severity: "must", Tool: "conftest",
			Repo: "acme", Target: "acme:infra/prod", File: "main.tf", Message: "bucket name must be snake_case", Status: "open"},
		{CheckID: "org.secrets", Layer: "org", Kind: "secrets", Stage: "checks", Severity: "must", Tool: "gitleaks",
			Repo: "acme", Message: "gitleaks is not installed (not on PATH); check org.secrets did not run", Status: "open"},
		{CheckID: "repo.docs", Layer: "repo", Kind: "docs", Stage: "checks", Severity: "may", Tool: "builtin",
			Repo: "acme", File: "README.md", Line: 3, Message: "missing changelog entry", Status: "waived"},
		{CheckID: "go.format", Layer: "org", Kind: "format", Stage: "checks", Severity: "must", Tool: "gofmt",
			Repo: "acme", File: "x.go", Message: "not gofmt-ed", Status: "fixed", AutofixApplied: true},
	}
}

type sarifDoc struct {
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				Rules   []struct {
					ID                   string `json:"id"`
					DefaultConfiguration struct {
						Level string `json:"level"`
					} `json:"defaultConfiguration"`
				} `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []struct {
			RuleID  string `json:"ruleId"`
			Level   string `json:"level"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI       string `json:"uri"`
						URIBaseID string `json:"uriBaseId"`
					} `json:"artifactLocation"`
					Region *struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
			Suppressions        []struct{ Kind string } `json:"suppressions"`
			PartialFingerprints map[string]string       `json:"partialFingerprints"`
		} `json:"results"`
	} `json:"runs"`
}

func TestSARIFValidatesAgainstTheOfficialSchema(t *testing.T) {
	data, err := buildSARIF("1.2.3", sarifFixtureFindings())
	if err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.Join("..", "..", "testdata", "sarif", "sarif-schema-2.1.0.json")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.Draft = jsonschema.Draft4
	if err := c.AddResource("sarif.json", bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("sarif.json")
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(v); err != nil {
		t.Errorf("SARIF does not validate: %#v\n%s", err, data)
	}
}

func TestSARIFContent(t *testing.T) {
	data, err := buildSARIF("1.2.3", sarifFixtureFindings())
	if err != nil {
		t.Fatal(err)
	}
	var doc sarifDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != "2.1.0" || len(doc.Runs) != 1 {
		t.Fatalf("version %q, %d runs", doc.Version, len(doc.Runs))
	}
	run := doc.Runs[0]
	if run.Tool.Driver.Name != "ai-flow" || run.Tool.Driver.Version != "1.2.3" {
		t.Errorf("driver = %+v", run.Tool.Driver)
	}

	var ruleIDs []string
	levels := map[string]string{}
	for _, r := range run.Tool.Driver.Rules {
		ruleIDs = append(ruleIDs, r.ID)
		levels[r.ID] = r.DefaultConfiguration.Level
	}
	// Sorted, one per check that has a reported result; the fixed
	// go.format finding contributes neither a result nor a rule.
	if want := []string{"go.lint", "infra.terraform.policy", "org.secrets", "repo.docs"}; !slices.Equal(ruleIDs, want) {
		t.Errorf("rules = %v, want %v", ruleIDs, want)
	}
	if levels["go.lint"] != "warning" || levels["org.secrets"] != "error" || levels["repo.docs"] != "note" {
		t.Errorf("rule levels = %v", levels)
	}

	type got struct {
		rule, uri, level string
		line             int
		suppressed       bool
	}
	var results []got
	for _, r := range run.Results {
		if len(r.Locations) != 1 {
			t.Fatalf("%s: %d locations, want 1", r.RuleID, len(r.Locations))
		}
		loc := r.Locations[0].PhysicalLocation
		if loc.ArtifactLocation.URIBaseID != "%SRCROOT%" {
			t.Errorf("%s: uriBaseId = %q", r.RuleID, loc.ArtifactLocation.URIBaseID)
		}
		line := 0
		if loc.Region != nil {
			line = loc.Region.StartLine
		}
		if r.PartialFingerprints["aiFlowFinding/v1"] == "" {
			t.Errorf("%s: no aiFlowFinding/v1 fingerprint", r.RuleID)
		}
		results = append(results, got{r.RuleID, loc.ArtifactLocation.URI, r.Level, line, len(r.Suppressions) == 1 && r.Suppressions[0].Kind == "external"})
	}
	// Sorted by uri, then line, then rule, then message.
	want := []got{
		{"org.secrets", ".", "error", 0, false},
		{"repo.docs", "README.md", "note", 3, true},
		{"go.lint", "cmd/main.go", "warning", 12, false},
		{"infra.terraform.policy", "infra/prod/main.tf", "error", 0, false},
	}
	if !slices.Equal(results, want) {
		t.Errorf("results =\n%+v\nwant\n%+v", results, want)
	}
}

// TestSARIFIsByteStable: the order findings arrive in never changes
// the bytes (Operational rules: same inputs, same bytes).
func TestSARIFIsByteStable(t *testing.T) {
	calls := 0
	handofftest.RequireByteStable(t, func() ([]byte, error) {
		f := sarifFixtureFindings()
		if calls++; calls%2 == 0 {
			slices.Reverse(f)
		}
		return buildSARIF("1.2.3", f)
	})
}

// TestSARIFNoFindings is a valid, empty run: a passing CI still uploads
// a file, which is how code scanning closes alerts that were fixed.
func TestSARIFNoFindings(t *testing.T) {
	data, err := buildSARIF("1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc sarifDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Runs) != 1 || doc.Runs[0].Results == nil || len(doc.Runs[0].Results) != 0 {
		t.Errorf("want one run with an empty results array:\n%s", data)
	}
}
