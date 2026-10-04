package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff/handofftest"
)

const waiversFixture = `{
  "schema_version": 1,
  "waivers": [
    {"check_id": "go.lint", "reason": "legacy package", "author": "ana", "created": "2026-09-01", "expires": "2026-09-30"},
    {"check_id": "go.format", "reason": "generated code", "author": "ben", "created": "2026-09-20", "expires": "2026-10-10"},
    {"check_id": "iac.scan", "reason": "vendor module", "author": "ana", "created": "2026-09-25", "expires": "2026-10-24"},
    {"check_id": "go.docs", "reason": "later", "author": "ben", "created": "2026-09-25", "expires": "2027-06-01"}
  ]
}
`

type waiversListEnvelope struct {
	OK     bool `json:"ok"`
	Result struct {
		Dir        string `json:"dir"`
		WindowDays int    `json:"window_days"`
		Active     int    `json:"active"`
		Expiring   int    `json:"expiring"`
		Expired    int    `json:"expired"`
		Waivers    []struct {
			CheckID  string `json:"check_id"`
			Expires  string `json:"expires"`
			State    string `json:"state"`
			DaysLeft int    `json:"days_left"`
		} `json:"waivers"`
	} `json:"result"`
	Error *struct{ Code, Message string } `json:"error"`
}

func runWaiversList(t *testing.T, wantExit int, args ...string) waiversListEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, append([]string{"standards", "waivers", "list"}, args...)...)
	if ec != wantExit {
		t.Fatalf("%v: exitCode = %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	var env waiversListEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("%v: stdout is not valid JSON: %v\n%s", args, err, stdout.String())
	}
	return env
}

func TestStandardsWaiversListClassifiesEveryWaiver(t *testing.T) {
	handofftest.FixedClock(t, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	repo := t.TempDir()
	writeFileTest(t, filepath.Join(repo, ".ai-flow", "waivers.json"), waiversFixture)

	env := runWaiversList(t, 0, repo, "--home", t.TempDir())
	r := env.Result
	if r.Dir != repo || r.WindowDays != 14 {
		t.Errorf("dir/window = %q/%d, want %q/14", r.Dir, r.WindowDays, repo)
	}
	if r.Expired != 1 || r.Expiring != 1 || r.Active != 2 || len(r.Waivers) != 4 {
		t.Fatalf("counts expired/expiring/active/total = %d/%d/%d/%d, want 1/1/2/4: %+v", r.Expired, r.Expiring, r.Active, len(r.Waivers), r.Waivers)
	}
	want := map[string]struct {
		state string
		days  int
	}{
		"go.lint":   {"expired", -4},
		"go.format": {"expiring", 5},
		"iac.scan":  {"active", 19},
		"go.docs":   {"active", 239},
	}
	for _, w := range r.Waivers {
		if got := want[w.CheckID]; w.State != got.state || w.DaysLeft != got.days {
			t.Errorf("%s: %s/%d, want %s/%d", w.CheckID, w.State, w.DaysLeft, got.state, got.days)
		}
	}
}

func TestStandardsWaiversListExpiringFiltersAndWidensTheWindow(t *testing.T) {
	handofftest.FixedClock(t, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	repo := t.TempDir()
	writeFileTest(t, filepath.Join(repo, ".ai-flow", "waivers.json"), waiversFixture)

	env := runWaiversList(t, 0, repo, "--expiring", "30", "--home", t.TempDir())
	r := env.Result
	if r.WindowDays != 30 {
		t.Errorf("window_days = %d, want 30", r.WindowDays)
	}
	got := map[string]string{}
	for _, w := range r.Waivers {
		got[w.CheckID] = w.State
	}
	want := map[string]string{"go.lint": "expired", "go.format": "expiring", "iac.scan": "expiring"}
	if len(got) != len(want) {
		t.Fatalf("listed %v, want exactly %v (the active go.docs waiver must be filtered out)", got, want)
	}
	for id, state := range want {
		if got[id] != state {
			t.Errorf("%s = %q, want %q", id, got[id], state)
		}
	}
}

func TestStandardsWaiversListNoFileIsEmpty(t *testing.T) {
	env := runWaiversList(t, 0, t.TempDir(), "--home", t.TempDir())
	if env.Result.Waivers == nil || len(env.Result.Waivers) != 0 {
		t.Errorf("waivers = %v, want an empty (non-null) list", env.Result.Waivers)
	}
}

func TestStandardsWaiversListMissingDirIsUsage(t *testing.T) {
	env := runWaiversList(t, 2, filepath.Join(t.TempDir(), "nope"), "--home", t.TempDir())
	if env.Error == nil || env.Error.Code != "usage" {
		t.Fatalf("error = %+v, want usage", env.Error)
	}
}

func TestStandardsWaiversListRejectsNegativeWindow(t *testing.T) {
	env := runWaiversList(t, 2, t.TempDir(), "--expiring", "-3", "--home", t.TempDir())
	if env.Error == nil || env.Error.Code != "usage" {
		t.Fatalf("error = %+v, want usage", env.Error)
	}
}
