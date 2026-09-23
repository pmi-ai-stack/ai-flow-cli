package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// waivableTicket is standardsTicket plus a repo-layer pack carrying
// two checks: one waivable (`dependency`) and one that never is
// (`secrets`). The reference pack ships no checks until phase 14, so
// this is where a resolvable check id comes from today.
func waivableTicket(t *testing.T) (home, repoDir string) {
	t.Helper()
	home, repoDir = standardsTicket(t, "PROJ-1")
	repoPack(t, repoDir, `{
  "schema_version": 1, "id": "acme.billing.api", "version": "0.1.0",
  "sections": [],
  "checks": [
    { "id": "repo.deps", "kind": "dependency", "severity": "must" },
    { "id": "repo.secrets", "kind": "secrets", "tool": "gitleaks", "severity": "must" }
  ]
}`)
	return home, repoDir
}

// TestWaiveWithoutConfirmedExitsFourWithResumeWith is the spec's own
// "exit 4 `waiver_confirmation`, then `--confirmed`" — and proves
// needs_input.resume_with is literally replayable, which is the
// contract the calling skill depends on.
func TestWaiveWithoutConfirmedExitsFourWithResumeWith(t *testing.T) {
	home, repoDir := waivableTicket(t)

	env := runStandards(t, 4,
		"standards", "waive", "PROJ-1",
		"--check", "repo.deps", "--repo", "acme.billing.api",
		"--reason", "the pinned upgrade lands in PROJ-9", "--expires", "2099-12-01",
		"--home", home)

	if env.Error == nil || env.Error.Code != "waiver_confirmation" {
		t.Fatalf("error = %+v, want code waiver_confirmation", env.Error)
	}
	if env.NeedsInput == nil {
		t.Fatal("needs_input is nil; exit 4 must carry the question it is asking")
	}
	if !strings.Contains(env.NeedsInput.ResumeWith, "--confirmed") {
		t.Errorf("resume_with = %q, want it to add --confirmed", env.NeedsInput.ResumeWith)
	}
	for _, want := range []string{"standards waive PROJ-1", "--check repo.deps", "--repo acme.billing.api", "--expires 2099-12-01"} {
		if !strings.Contains(env.NeedsInput.ResumeWith, want) {
			t.Errorf("resume_with = %q, want it to contain %q", env.NeedsInput.ResumeWith, want)
		}
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".ai-flow", "waivers.json")); err == nil {
		t.Error("waivers.json was written despite the missing --confirmed")
	}
}

// TestWaiveWritesWaiversJSON proves the confirmed path: the file
// lands in the repository, and the very next resolve reports the check
// as waived.
func TestWaiveWritesWaiversJSON(t *testing.T) {
	home, repoDir := waivableTicket(t)

	before := runStandards(t, 0, "standards", "resolve", "PROJ-1", "--home", home)
	if len(before.Result.Waived) != 0 {
		t.Fatalf("waived = %v before any waiver exists, want none", before.Result.Waived)
	}

	env := runStandards(t, 0,
		"standards", "waive", "PROJ-1",
		"--check", "repo.deps", "--repo", "acme.billing.api",
		"--reason", "the pinned upgrade lands in PROJ-9", "--expires", "2099-12-01", "--confirmed",
		"--home", home)
	if !env.OK {
		t.Fatalf("ok = false: %+v", env.Error)
	}

	data, err := os.ReadFile(filepath.Join(repoDir, ".ai-flow", "waivers.json"))
	if err != nil {
		t.Fatalf("ReadFile waivers.json: %v", err)
	}
	var file struct {
		SchemaVersion int `json:"schema_version"`
		Waivers       []struct {
			CheckID string `json:"check_id"`
			Reason  string `json:"reason"`
			Author  string `json:"author"`
			Created string `json:"created"`
			Expires string `json:"expires"`
		} `json:"waivers"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("waivers.json is not valid JSON: %v\n%s", err, data)
	}
	if file.SchemaVersion != 1 || len(file.Waivers) != 1 {
		t.Fatalf("waivers.json = %+v, want schema_version 1 and exactly one waiver", file)
	}
	w := file.Waivers[0]
	if w.CheckID != "repo.deps" || w.Reason == "" || w.Author == "" || w.Created == "" {
		t.Errorf("waiver = %+v, want every field populated", w)
	}
	if w.Expires != "2099-12-01T00:00:00Z" {
		t.Errorf("expires = %q, want the bare date normalized to midnight UTC", w.Expires)
	}

	after := runStandards(t, 0, "standards", "resolve", "PROJ-1", "--home", home)
	if len(after.Result.Waived) != 1 || after.Result.Waived[0] != "repo.deps" {
		t.Errorf("waived = %v, want [repo.deps]", after.Result.Waived)
	}
	if after.Result.ResolutionHash == before.Result.ResolutionHash {
		t.Error("the resolution hash did not move after a new waiver; waivers are one of its documented inputs")
	}
}

// TestSecretsCheckCannotBeWaived is the spec's flat rule. It is
// `usage` (exit 2) rather than exit 4: the CLI detects it before
// touching anything, which is exit 2's own definition.
func TestSecretsCheckCannotBeWaived(t *testing.T) {
	home, repoDir := waivableTicket(t)

	env := runStandards(t, 2,
		"standards", "waive", "PROJ-1",
		"--check", "repo.secrets", "--repo", "acme.billing.api",
		"--reason", "it is noisy", "--expires", "2099-12-01", "--confirmed",
		"--home", home)

	if env.Error == nil || env.Error.Code != "usage" {
		t.Fatalf("error = %+v, want code usage", env.Error)
	}
	if !strings.Contains(env.Error.Message, "secrets") {
		t.Errorf("message = %q, want it to name the secrets kind", env.Error.Message)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".ai-flow", "waivers.json")); err == nil {
		t.Error("waivers.json was written for a secrets check")
	}
}

// TestSectionsCannotBeWaived proves the other half of the same spec
// line — "sections cannot be waived, only overridden" — by naming a
// resolved *section* id where a check id belongs.
func TestSectionsCannotBeWaived(t *testing.T) {
	home, _ := waivableTicket(t)

	env := runStandards(t, 2,
		"standards", "waive", "PROJ-1",
		"--check", "go.errors", "--repo", "acme.billing.api",
		"--reason", "r", "--expires", "2099-12-01", "--confirmed",
		"--home", home)

	if env.Error == nil || env.Error.Code != "usage" {
		t.Fatalf("error = %+v, want code usage", env.Error)
	}
	var said bool
	for _, d := range env.Error.Details {
		if strings.Contains(d, "sections cannot be waived") {
			said = true
		}
	}
	if !said {
		t.Errorf("details = %v, want them to say sections cannot be waived", env.Error.Details)
	}
}

// TestWaiveRefusesABadExpiry covers the required, future, parseable
// expiry — a waiver without one is a silent permanent suppression.
func TestWaiveRefusesABadExpiry(t *testing.T) {
	home, _ := waivableTicket(t)

	base := []string{
		"standards", "waive", "PROJ-1",
		"--check", "repo.deps", "--repo", "acme.billing.api", "--reason", "r", "--confirmed", "--home", home,
	}

	tests := map[string]struct {
		args []string
		want string
	}{
		"missing":      {base, "--expires"},
		"in the past":  {append(append([]string{}, base...), "--expires", "2020-01-01"), "not in the future"},
		"unparseable":  {append(append([]string{}, base...), "--expires", "next tuesday"), "--expires"},
		"unknown repo": {append(append([]string{}, base...), "--expires", "2099-12-01", "--repo", "acme.nope"), "acme.nope"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			env := runStandards(t, 2, tc.args...)
			if env.Error == nil || env.Error.Code != "usage" {
				t.Fatalf("error = %+v, want code usage", env.Error)
			}
			if !strings.Contains(env.Error.Message, tc.want) {
				t.Errorf("message = %q, want it to mention %q", env.Error.Message, tc.want)
			}
		})
	}
}
