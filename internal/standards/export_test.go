package standards

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExportRoundTripsToAValidatingDirectory is the phase's own "Done
// when": standards export writes a directory standards validate then
// accepts, against the real embedded reference pack — the same round
// trip a developer forking the reference pack would perform.
func TestExportRoundTripsToAValidatingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reference")
	if err := Export(dir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
		t.Fatalf("exported dir has no %s: %v", ManifestFile, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "org", "guidance", "errors.md")); err != nil {
		t.Fatalf("exported dir missing org/guidance/errors.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "languages", "go", "checks", ".editorconfig")); err != nil {
		t.Fatalf("exported dir missing a dotfile under languages/go/checks/ — all: embed prefix regressed: %v", err)
	}

	if _, err := ValidateAgainstProductionAllowlist(dir); err != nil {
		t.Fatalf("Validate(exported dir): %v", err)
	}
}

func TestExportRefusesNonEmptyDestination(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "already-here.txt"), "hello")

	err := Export(dir)
	if err == nil {
		t.Fatal("Export into a non-empty dir = nil error, want a refusal")
	}
}

func TestExportIntoFreshOrEmptyDirSucceeds(t *testing.T) {
	// Empty-but-existing is as valid a target as nonexistent.
	dir := t.TempDir()
	if err := Export(dir); err != nil {
		t.Fatalf("Export into an empty existing dir: %v", err)
	}
}
