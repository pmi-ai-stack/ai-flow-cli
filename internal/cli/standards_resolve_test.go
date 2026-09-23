package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// standardsTicket lays out everything a ticket-scoped standards
// command needs: an initialized home (so config.json exists with the
// template's own guidance_budget_tokens), a repo.json naming one
// repository, and a real checkout for it carrying an AGENTS.md. It
// returns the home and that checkout's path.
//
// Deliberately no config.standards.sources: the embedded reference
// pack is then the org layer, which is both the default a fresh
// `config init` produces and the one layer set that needs no network
// and no sync cache.
func standardsTicket(t *testing.T, key string) (home, repoDir string) {
	t.Helper()
	home = t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	repoDir = t.TempDir()
	writeFileTest(t, filepath.Join(repoDir, "AGENTS.md"), "# acme.billing.api\n\nWrap every error with %w.\n")
	writeFileTest(t, filepath.Join(repoDir, "go.mod"), "module example.com/acme\n\ngo 1.27\n")

	writeRepoFixture(t, filepath.Join(home, key), "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: repoDir, InstructionsFile: "AGENTS.md"},
	})
	return home, repoDir
}

// repoPack installs a repo-layer pack into repoDir, with the given
// manifest body.
func repoPack(t *testing.T, repoDir, manifest string) {
	t.Helper()
	dir := filepath.Join(repoDir, ".ai-flow", "standards")
	writeFileTest(t, filepath.Join(dir, "guidance", "errors.md"), "# Repo errors\n\nWrap with the platform trace type.\n")
	writeFileTest(t, filepath.Join(dir, "standards.json"), manifest)
}

type resolveEnvelope struct {
	OK     bool `json:"ok"`
	Result struct {
		Key            string   `json:"key"`
		ResolutionHash string   `json:"resolution_hash"`
		Unchanged      bool     `json:"unchanged"`
		Languages      []string `json:"languages"`
		Sections       int      `json:"sections"`
		Checks         int      `json:"checks"`
		Waived         []string `json:"waived"`
		Conflicts      []struct {
			ID     string `json:"id"`
			Rule   string `json:"rule"`
			Winner string `json:"winner"`
			Loser  string `json:"loser"`
		} `json:"conflicts"`
	} `json:"result"`
	Error *struct {
		Code    string   `json:"code"`
		Message string   `json:"message"`
		Details []string `json:"details"`
	} `json:"error"`
	NeedsInput *struct {
		Question   string   `json:"question"`
		Options    []string `json:"options"`
		ResumeWith string   `json:"resume_with"`
	} `json:"needs_input"`
	Warnings []string `json:"warnings"`
}

func runStandards(t *testing.T, wantExit int, args ...string) resolveEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, args...)
	if ec != wantExit {
		t.Fatalf("%v: exitCode = %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	var env resolveEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("%v: stdout is not valid JSON: %v\n%s", args, err, stdout.String())
	}
	return env
}

// TestResolveIdempotentOnHash is the spec's "`resolve` is idempotent
// on the resolution hash": the second run reports unchanged and leaves
// standards.json byte for byte as it was.
func TestResolveIdempotentOnHash(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")

	first := runStandards(t, 0, "standards", "resolve", "PROJ-1", "--home", home)
	if first.Result.Unchanged {
		t.Error("the first resolve reports unchanged, want false")
	}
	if first.Result.ResolutionHash == "" {
		t.Error("the first resolve reports no resolution hash")
	}
	if first.Result.Sections == 0 {
		t.Error("the first resolve reports no sections; the embedded reference pack should have contributed")
	}

	path := filepath.Join(home, "PROJ-1", "standards.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile standards.json: %v", err)
	}

	second := runStandards(t, 0, "standards", "resolve", "PROJ-1", "--home", home)
	if !second.Result.Unchanged {
		t.Error("the second resolve reports unchanged = false, want true")
	}
	if second.Result.ResolutionHash != first.Result.ResolutionHash {
		t.Errorf("hash moved between two identical runs: %s -> %s", first.Result.ResolutionHash, second.Result.ResolutionHash)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile standards.json: %v", err)
	}
	if string(before) != string(after) {
		t.Error("standards.json changed across two identical resolves")
	}
}

// TestResolveWithoutRepoJSONIsMissingArtifact proves the direct
// command still refuses before `repo locate` has run — only `stage
// start` skips silently.
func TestResolveWithoutRepoJSONIsMissingArtifact(t *testing.T) {
	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	env := runStandards(t, 3, "standards", "resolve", "PROJ-1", "--home", home)
	if env.Error == nil || env.Error.Code != "missing_artifact" {
		t.Fatalf("error = %+v, want code missing_artifact", env.Error)
	}
}

// TestResolveReportsAnUndeclaredOverrideConflict is the CLI-level view
// of the precedence rule: the org entry wins, the conflict is in the
// result and in the envelope's warnings, and the command still exits 0.
func TestResolveReportsAnUndeclaredOverrideConflict(t *testing.T) {
	home, repoDir := standardsTicket(t, "PROJ-1")
	repoPack(t, repoDir, `{
  "schema_version": 1, "id": "acme.billing.api", "version": "0.1.0",
  "sections": [
    { "id": "go.errors", "path": "guidance/errors.md", "severity": "must",
      "applies_to": { "languages": ["go"], "stages": ["implement", "review"] } }
  ],
  "checks": []
}`)

	env := runStandards(t, 0, "standards", "resolve", "PROJ-1", "--home", home)
	if !env.OK {
		t.Fatal("ok = false, want true: a conflict is reported, not fatal")
	}
	if len(env.Result.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want exactly one", env.Result.Conflicts)
	}
	c := env.Result.Conflicts[0]
	if c.ID != "go.errors" || c.Rule != "override_not_declared" || c.Winner != "org" || c.Loser != "repo" {
		t.Errorf("conflict = %+v, want go.errors override_not_declared org/repo", c)
	}

	var warned bool
	for _, w := range env.Warnings {
		if strings.Contains(w, "go.errors") && strings.Contains(w, "overrides[]") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want one naming the undeclared override", env.Warnings)
	}
}

// TestResolveAcceptsADeclaredOverride is the other half: with an
// overrides[] entry the repo pack wins, the reason reaches the
// envelope's warnings, and no conflict is reported.
func TestResolveAcceptsADeclaredOverride(t *testing.T) {
	home, repoDir := standardsTicket(t, "PROJ-1")
	repoPack(t, repoDir, `{
  "schema_version": 1, "id": "acme.billing.api", "version": "0.1.0",
  "sections": [
    { "id": "go.errors", "path": "guidance/errors.md", "severity": "must",
      "applies_to": { "languages": ["go"], "stages": ["implement", "review"] } }
  ],
  "checks": [],
  "overrides": [
    { "id": "go.errors", "reason": "this service carries trace context on every error" }
  ]
}`)

	env := runStandards(t, 0, "standards", "resolve", "PROJ-1", "--home", home)
	if len(env.Result.Conflicts) != 0 {
		t.Errorf("conflicts = %+v, want none; the override was declared", env.Result.Conflicts)
	}

	var warned bool
	for _, w := range env.Warnings {
		if strings.Contains(w, "go.errors") && strings.Contains(w, "trace context") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want the declared override's reason reported on every resolve", env.Warnings)
	}

	show := showSection(t, home, "PROJ-1", "go.errors")
	if show.Layer != "repo" {
		t.Errorf("go.errors layer = %q, want repo", show.Layer)
	}
	if !strings.Contains(show.Content, "platform trace type") {
		t.Errorf("go.errors content = %q, want the repo pack's own body", show.Content)
	}
}

type showResultEnvelope struct {
	ID          string   `json:"id"`
	Layer       string   `json:"layer"`
	Severity    string   `json:"severity"`
	Path        string   `json:"path"`
	DroppedFrom []string `json:"dropped_from"`
	Content     string   `json:"content"`
}

func showSection(t *testing.T, home, key, id string) showResultEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, "standards", "show", key, "--id", id, "--home", home)
	if ec != 0 {
		t.Fatalf("standards show %s: exitCode = %d\n%s", id, ec, stdout.String())
	}
	var env struct {
		Result showResultEnvelope `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	return env.Result
}

// TestStandardsShowPrintsDroppedSection is the spec's "`standards show
// KEY --id <id>` prints a dropped section on demand" — the whole point
// of the command is that the budget having dropped a section from a
// stage's guidance.md does not put it out of reach.
func TestStandardsShowPrintsDroppedSection(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")
	runStandards(t, 0, "standards", "resolve", "PROJ-1", "--home", home)

	// go.generics is the reference pack's one `may` section, and the
	// template's 6000-token budget is far below the Go guidance's total
	// size, so it is dropped from every stage it applies to.
	got := showSection(t, home, "PROJ-1", "go.generics")
	if got.Severity != "may" {
		t.Errorf("go.generics severity = %q, want may", got.Severity)
	}
	if len(got.DroppedFrom) == 0 {
		t.Error("go.generics dropped_from is empty; the reference pack's may section should not survive a 6000-token budget")
	}
	if !strings.Contains(got.Content, "Generics") {
		t.Errorf("go.generics content does not look like the section's own file:\n%s", got.Content)
	}
}

// TestStandardsShowUnknownIDIsUsage proves the unknown-id error names
// what would have been accepted, the same shape `schema <artifact>`
// and `stage <stage>` already use.
func TestStandardsShowUnknownIDIsUsage(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-1")

	env := runStandards(t, 2, "standards", "show", "PROJ-1", "--id", "no.such.section", "--home", home)
	if env.Error == nil || env.Error.Code != "usage" {
		t.Fatalf("error = %+v, want code usage", env.Error)
	}
	if len(env.Error.Details) == 0 {
		t.Error("error.details is empty, want every acceptable section id")
	}

	env = runStandards(t, 2, "standards", "show", "PROJ-1", "--home", home)
	if env.Error == nil || !strings.Contains(env.Error.Message, "--id") {
		t.Errorf("error = %+v, want a message naming the required --id flag", env.Error)
	}
}
