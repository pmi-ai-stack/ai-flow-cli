package handoff

import (
	"testing"
	"time"
)

// fixedClock pins the package-level Now clock for the duration of a
// test. It duplicates handofftest.FixedClock's logic rather than
// importing that package: handofftest imports internal/handoff, so an
// internal (same-package) test file here importing it back would be an
// import cycle — the exact reason handofftest exists as a separate
// package other packages' tests can safely import instead.
func fixedClock(tb testing.TB, at time.Time) {
	tb.Helper()
	orig := Now
	Now = func() time.Time { return at }
	tb.Cleanup(func() { Now = orig })
}

func TestLoadActiveMissingFile(t *testing.T) {
	home := t.TempDir()
	a, err := LoadActive(home)
	if err != nil {
		t.Fatalf("LoadActive: %v", err)
	}
	if a != nil {
		t.Errorf("a = %+v, want nil for no active ticket", a)
	}
}

func TestRecordActiveRoundTrip(t *testing.T) {
	home := t.TempDir()
	fixed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fixedClock(t, fixed)

	if err := RecordActive(home, "PROJ-1", StagePlan); err != nil {
		t.Fatalf("RecordActive: %v", err)
	}

	a, err := LoadActive(home)
	if err != nil {
		t.Fatalf("LoadActive: %v", err)
	}
	if a == nil {
		t.Fatal("LoadActive returned nil after RecordActive")
	}
	if a.Key != "PROJ-1" || a.Stage != "plan" {
		t.Errorf("a = %+v, want key=PROJ-1 stage=plan", a)
	}
	if a.LastEventAt != "2026-01-01T12:00:00Z" {
		t.Errorf("LastEventAt = %q, want the fixed clock's RFC3339 form", a.LastEventAt)
	}
	if a.AllowlistHash == "" {
		t.Error("AllowlistHash must not be empty")
	}
	if a.RepoRoots == nil || a.ToolConfigFiles == nil {
		t.Error("RepoRoots/ToolConfigFiles should be empty slices, not nil, until phases 5/14 populate them")
	}
}

// TestRecordActiveIsSingularNotPerTicket proves design decision 7:
// recording a second ticket overwrites the first, system-wide.
func TestRecordActiveIsSingularNotPerTicket(t *testing.T) {
	home := t.TempDir()
	if err := RecordActive(home, "PROJ-1", StageFetch); err != nil {
		t.Fatalf("RecordActive: %v", err)
	}
	if err := RecordActive(home, "PROJ-2", StageLocate); err != nil {
		t.Fatalf("RecordActive: %v", err)
	}
	a, err := LoadActive(home)
	if err != nil {
		t.Fatalf("LoadActive: %v", err)
	}
	if a.Key != "PROJ-2" {
		t.Errorf("Key = %q, want PROJ-2 (the most recent record, singular active.json)", a.Key)
	}
}
