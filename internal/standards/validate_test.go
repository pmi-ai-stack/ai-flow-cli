package standards

import (
	"errors"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func TestValidateAcceptsValidFixture(t *testing.T) {
	res, err := Validate("testdata/packs/valid", []string{"go"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Manifest.ID != "fixture" {
		t.Errorf("ID = %q, want %q", res.Manifest.ID, "fixture")
	}
}

func TestValidateCheckRunnerMissing(t *testing.T) {
	_, err := Validate("testdata/packs/invalid-check-runner-missing", nil)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("Validate error type = %T, want *envelope.Error", err)
	}
	if envErr.Code != "check_runner_missing" {
		t.Errorf("Code = %q, want %q", envErr.Code, "check_runner_missing")
	}
	if len(envErr.Details) != 1 {
		t.Errorf("Details = %v, want exactly one violation", envErr.Details)
	}
}

func TestValidateLintNamesRenderingValidator(t *testing.T) {
	// tflint IS in the fixture allowlist here, isolating the
	// rendering-validator rule from check_runner_missing so the test
	// proves each rule fires independently.
	_, err := Validate("testdata/packs/invalid-lint-names-render-validator", []string{"tflint"})
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("Validate error type = %T, want *envelope.Error", err)
	}
	if envErr.Code != "pack_invalid" {
		t.Errorf("Code = %q, want %q (not check_runner_missing, since tflint is allowed here)", envErr.Code, "pack_invalid")
	}
	if len(envErr.Details) != 1 {
		t.Errorf("Details = %v, want exactly one violation", envErr.Details)
	}
}

func TestValidateMissingSectionFile(t *testing.T) {
	_, err := Validate("testdata/packs/invalid-missing-section-file", nil)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("Validate error type = %T, want *envelope.Error", err)
	}
	if envErr.Code != "pack_invalid" {
		t.Errorf("Code = %q, want %q", envErr.Code, "pack_invalid")
	}
}

func TestValidateCheckRunnerMissingWinsOverPackInvalid(t *testing.T) {
	// invalid-missing-section-file's own section path is broken (a
	// pack_invalid-only violation); adding an unresolvable tool
	// (check_runner_missing) to the same manifest's checks[] must still
	// win the top-level code, per design decision 7 — both violations
	// still land in Details.
	dir := t.TempDir()
	writeFile(t, dir+"/standards.json", `{
  "schema_version": 1, "id": "fixture", "version": "1.0.0",
  "sections": [{ "id": "go.errors", "path": "missing.md", "severity": "must" }],
  "checks": [{ "id": "x", "kind": "scan", "tool": "nope", "severity": "must" }]
}`)
	_, err := Validate(dir, nil)
	var envErr *envelope.Error
	if !errors.As(err, &envErr) {
		t.Fatalf("Validate error type = %T, want *envelope.Error", err)
	}
	if envErr.Code != "check_runner_missing" {
		t.Errorf("Code = %q, want %q", envErr.Code, "check_runner_missing")
	}
	if len(envErr.Details) != 2 {
		t.Errorf("Details = %v, want two violations", envErr.Details)
	}
}

func TestValidateAgainstProductionAllowlistOnReferencePack(t *testing.T) {
	// The phase's own "Done when": the real embedded reference pack
	// validates against the real production allowlist. Exercised again,
	// end to end via Export, in export_test.go.
	if _, err := ValidateAgainstProductionAllowlist("../../packs/reference"); err != nil {
		t.Fatalf("ValidateAgainstProductionAllowlist(packs/reference): %v", err)
	}
}
