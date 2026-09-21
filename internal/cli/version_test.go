package cli

import "testing"

func TestBuildVersionResultUsesInjectedValues(t *testing.T) {
	bi := BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-09-21T00:00:00Z"}
	got := buildVersionResult(bi, "go1.27.1")

	want := versionResult{
		Version:       "1.2.3",
		Commit:        "abc123",
		Date:          "2026-09-21T00:00:00Z",
		GoVersion:     "go1.27.1",
		SchemaVersion: 1,
	}
	if got != want {
		t.Errorf("buildVersionResult() = %+v, want %+v", got, want)
	}
}

func TestBuildVersionResultDefaultsVersionToDev(t *testing.T) {
	got := buildVersionResult(BuildInfo{}, "go1.27.1")
	if got.Version != "dev" {
		t.Errorf("Version = %q, want %q", got.Version, "dev")
	}
	// Commit/date fall back through debug.ReadBuildInfo() and, failing
	// that, "unknown" — either is acceptable under `go test`, but never
	// empty: a skill must never see a blank field.
	if got.Commit == "" {
		t.Error("Commit is empty, want a fallback value")
	}
	if got.Date == "" {
		t.Error("Date is empty, want a fallback value")
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, SchemaVersion)
	}
}
