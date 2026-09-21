package schema

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestValidFixtureAccepted and TestInvalidFixtureRejected are table
// tests over every artifact in Artifacts(), reading
// testdata/<artifact>/{valid,invalid}.json — the "a valid and an
// invalid fixture per schema" the phase's plan requires.

func TestValidFixtureAccepted(t *testing.T) {
	for _, artifact := range Artifacts() {
		t.Run(artifact, func(t *testing.T) {
			data := readFixture(t, artifact, "valid.json")
			if err := Validate(artifact, data); err != nil {
				t.Errorf("Validate(%q, valid fixture) = %v, want nil", artifact, err)
			}
		})
	}
}

func TestInvalidFixtureRejected(t *testing.T) {
	for _, artifact := range Artifacts() {
		t.Run(artifact, func(t *testing.T) {
			data := readFixture(t, artifact, "invalid.json")
			err := Validate(artifact, data)
			if err == nil {
				t.Fatalf("Validate(%q, invalid fixture) = nil, want an error", artifact)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Validate(%q, invalid fixture) error type = %T, want *ValidationError", artifact, err)
			}
			if len(ve.Failures) == 0 {
				t.Errorf("Validate(%q, invalid fixture): ValidationError has no Failures", artifact)
			}
			if !slices.IsSortedFunc(ve.Failures, func(a, b Failure) int {
				return cmp.Compare(a.Pointer, b.Pointer)
			}) {
				t.Errorf("Validate(%q, invalid fixture): Failures not sorted by Pointer: %+v", artifact, ve.Failures)
			}
		})
	}
}

func TestValidateUnknownArtifact(t *testing.T) {
	err := Validate("no-such-artifact", []byte(`{}`))
	if !errors.Is(err, ErrUnknownArtifact) {
		t.Errorf("Validate(unknown artifact) = %v, want ErrUnknownArtifact", err)
	}
}

func TestValidateMalformedJSON(t *testing.T) {
	err := Validate("status", []byte(`{not json`))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate(malformed JSON) error type = %T, want *ValidationError", err)
	}
	if len(ve.Failures) != 1 || ve.Failures[0].Message == "" {
		t.Errorf("Validate(malformed JSON) Failures = %+v, want one failure with a message", ve.Failures)
	}
}

// TestAllHandoffArtifactsHaveASchema pins the 17 names against the
// spec's handoff table transcription in the plan, so a future rename
// there is caught here too.
func TestAllHandoffArtifactsHaveASchema(t *testing.T) {
	want := []string{
		"status", "standards", "ticket", "repo", "ops-targets", "test-results",
		"render-drift-results", "dryrun-results", "impact-results", "rollback-results",
		"infra-test-results", "checks-results", "mutation-results", "visual-test-results",
		"e2e-results", "ship-results", "skill-frontmatter",
	}
	got := Artifacts()
	if !slices.Equal(got, want) {
		t.Errorf("Artifacts() = %v, want %v", got, want)
	}
}

func TestPrintReturnsDecodedSchema(t *testing.T) {
	v, err := Print("status")
	if err != nil {
		t.Fatalf("Print(status): %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("Print(status) = %T, want map[string]any", v)
	}
	if m["title"] != "status" {
		t.Errorf(`Print(status)["title"] = %v, want "status"`, m["title"])
	}
}

func TestPrintUnknownArtifact(t *testing.T) {
	if _, err := Print("no-such-artifact"); !errors.Is(err, ErrUnknownArtifact) {
		t.Errorf("Print(unknown artifact) = %v, want ErrUnknownArtifact", err)
	}
}

func readFixture(t *testing.T, artifact, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", artifact, name))
	if err != nil {
		t.Fatalf("read fixture %s/%s: %v", artifact, name, err)
	}
	return data
}
