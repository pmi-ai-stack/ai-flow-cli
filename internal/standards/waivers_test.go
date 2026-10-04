package standards

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
)

func TestParseExpiryAcceptsBothSpellings(t *testing.T) {
	want := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	for _, in := range []string{"2026-12-01", "2026-12-01T00:00:00Z", "2026-12-01T01:00:00+01:00"} {
		got, err := ParseExpiry(in)
		if err != nil {
			t.Errorf("ParseExpiry(%q) = %v, want no error", in, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("ParseExpiry(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseExpiry("next tuesday"); err == nil {
		t.Error("ParseExpiry(\"next tuesday\") = nil error, want one")
	}
}

// TestExpiredTreatsAnUnreadableExpiryAsExpired pins the safe default:
// a waiver whose expiry cannot be parsed must not become a permanent
// suppression, which is exactly what treating it as "not expired"
// would make it.
func TestExpiredTreatsAnUnreadableExpiryAsExpired(t *testing.T) {
	now := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		expires string
		want    bool
	}{
		{"2099-01-01T00:00:00Z", false},
		{"2020-01-01T00:00:00Z", true},
		{"2026-09-22T00:00:00Z", true}, // the instant itself is already expired
		{"whenever", true},
		{"", true},
	} {
		if got := Expired(Waiver{Expires: tc.expires}, now); got != tc.want {
			t.Errorf("Expired(expires: %q) = %v, want %v", tc.expires, got, tc.want)
		}
	}
}

// TestLoadWaiversTolerantOfAMissingFile proves the "missing means not
// written yet, not an error" convention this codebase uses everywhere.
func TestLoadWaiversTolerantOfAMissingFile(t *testing.T) {
	got, err := LoadWaivers("acme.billing.api", t.TempDir())
	if err != nil {
		t.Fatalf("LoadWaivers on a repository with no waivers file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("LoadWaivers = %v, want none", got)
	}
}

// TestAddWaiverReplacesTheSameCheck proves a renewal never leaves the
// expired original behind to be read back alongside it.
func TestAddWaiverReplacesTheSameCheck(t *testing.T) {
	repoDir := stageRepo(t, "one", false)

	if err := AddWaiver(repoDir, Waiver{
		CheckID: "org.lint", Reason: "renewed", Author: "dev",
		Created: "2026-09-22T00:00:00Z", Expires: "2027-06-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("AddWaiver: %v", err)
	}

	got, err := LoadWaivers("acme.billing.api", repoDir)
	if err != nil {
		t.Fatalf("LoadWaivers: %v", err)
	}
	var lint []Waiver
	for _, w := range got {
		if w.CheckID == "org.lint" {
			lint = append(lint, w)
		}
	}
	if len(lint) != 1 {
		t.Fatalf("org.lint has %d waivers after a renewal, want exactly 1", len(lint))
	}
	if lint[0].Expires != "2027-06-01T00:00:00Z" {
		t.Errorf("org.lint expires = %q, want the renewed value", lint[0].Expires)
	}
	// The fixture's other waiver survives untouched.
	if len(got) != 2 {
		t.Errorf("waivers = %d, want 2 (the renewal plus the untouched org.deps)", len(got))
	}
	// Sorted by check_id, so the file is byte-stable across renewals.
	if got[0].CheckID != "org.deps" || got[1].CheckID != "org.lint" {
		t.Errorf("waivers are not sorted by check_id: %v", []string{got[0].CheckID, got[1].CheckID})
	}
}

// TestAddWaiverWritesASchemaValidFile proves the write goes through
// the schema, like every other machine-readable write in the system.
func TestAddWaiverWritesASchemaValidFile(t *testing.T) {
	repoDir := t.TempDir()
	if err := AddWaiver(repoDir, Waiver{
		CheckID: "org.deps", Reason: "the pinned upgrade lands in PROJ-9", Author: "dev",
		Created: "2026-09-22T00:00:00Z", Expires: "2026-12-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("AddWaiver: %v", err)
	}

	path := WaiversPath(repoDir)
	if want := filepath.Join(repoDir, ".ai-flow", "waivers.json"); path != want {
		t.Errorf("WaiversPath = %s, want %s", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := schema.Validate("waivers", data); err != nil {
		t.Errorf("the written waivers.json does not satisfy its own schema: %v", err)
	}
}

// TestAddWaiverRefusesAWaiverWithNoExpiry proves the schema, not just
// the CLI flag check, enforces the required expiry — a direct
// AddWaiver call cannot write one either.
func TestAddWaiverRefusesAWaiverWithNoExpiry(t *testing.T) {
	err := AddWaiver(t.TempDir(), Waiver{
		CheckID: "org.deps", Reason: "r", Author: "dev", Created: "2026-09-22T00:00:00Z",
	})
	if err == nil {
		t.Fatal("AddWaiver with no expires = nil error, want a schema failure")
	}
}

func TestExpiryState(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	window := 14 * 24 * time.Hour
	for _, tc := range []struct {
		name, expires, wantState string
		wantDays                 int
	}{
		{"far future", "2027-01-01", StateActive, 88},
		{"just outside the window", "2026-10-19", StateActive, 14},
		{"inside the window", "2026-10-10", StateExpiring, 5},
		{"last day of the window", "2026-10-18T12:00:00Z", StateExpiring, 14},
		{"later today", "2026-10-04T18:00:00Z", StateExpiring, 0},
		{"the instant itself", "2026-10-04T12:00:00Z", StateExpired, 0},
		{"yesterday", "2026-10-03", StateExpired, -1},
		{"long ago", "2026-09-01", StateExpired, -33},
		{"unreadable", "whenever", StateExpired, 0},
	} {
		state, days := ExpiryState(Waiver{Expires: tc.expires}, now, window)
		if state != tc.wantState || days != tc.wantDays {
			t.Errorf("%s: ExpiryState(%q) = %s, %d; want %s, %d", tc.name, tc.expires, state, days, tc.wantState, tc.wantDays)
		}
	}
}

// TestExpiryStateAgreesWithExpired: the two must never disagree about
// whether a waiver still suppresses its check.
func TestExpiryStateAgreesWithExpired(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, e := range []string{"2099-01-01", "2026-10-04T12:00:00Z", "2026-10-04T12:00:01Z", "2020-01-01", "x", ""} {
		w := Waiver{Expires: e}
		state, _ := ExpiryState(w, now, 14*24*time.Hour)
		if (state == StateExpired) != Expired(w, now) {
			t.Errorf("expires %q: ExpiryState says %s but Expired says %v", e, state, Expired(w, now))
		}
	}
}
