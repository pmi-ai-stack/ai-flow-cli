package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/gitops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// --- pure-function tests (no cobra, no subprocess) ---

func TestLatestMutationRoundNoDirectory(t *testing.T) {
	n, err := latestMutationRound(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("latestMutationRound(empty) = %d, want 0", n)
	}
}

func TestLatestMutationRoundPicksHighest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"round-1", "round-3", "round-2"} {
		if err := os.MkdirAll(filepath.Join(dir, "mutation-reports", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	n, err := latestMutationRound(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("latestMutationRound = %d, want 3", n)
	}
}

func TestRoundScore(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{100, 100}, {66.666, 66.7}, {0, 0}, {33.33, 33.3},
	}
	for _, c := range cases {
		if got := roundScore(c.in); got != c.want {
			t.Errorf("roundScore(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEmptyIfNil(t *testing.T) {
	if got := emptyIfNil[mutationResultsEquivalent](nil); len(got) != 0 || got == nil {
		t.Errorf("emptyIfNil(nil) = %#v, want non-nil empty slice", got)
	}
	in := []mutationResultsEquivalent{{ID: "1", Reason: "r"}}
	if got := emptyIfNil(in); len(got) != 1 {
		t.Errorf("emptyIfNil(in) = %#v", got)
	}
}

func TestLoadRoundReports(t *testing.T) {
	dir := t.TempDir()
	roundDir := filepath.Join(dir, "round-1")
	writeRawJSON(t, roundDir, "b.json", `{"tool":"gremlins","total":2,"killed":2,"survivors":[]}`)
	writeRawJSON(t, roundDir, "a.json", `{"tool":"mutmut","total":3,"killed":1,"survivors":[{"id":"1","file":"x.py","line":4}]}`)

	parsed, err := loadRoundReports(roundDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("loadRoundReports = %d entries, want 2", len(parsed))
	}
	// Sorted by repo name: "a" before "b".
	if parsed[0].repo != "a" || parsed[1].repo != "b" {
		t.Errorf("order = %v, want [a b]", []string{parsed[0].repo, parsed[1].repo})
	}
	if parsed[0].report.Tool != "mutmut" || parsed[0].report.Total != 3 {
		t.Errorf("parsed[0] = %+v", parsed[0].report)
	}
}

func writeRawJSON(t *testing.T, dir, filename, data string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// testMutationConfig builds a minimal valid *config.Config for the
// pure-function tests below — just the fields
// ensureMutationTargetsTestsGreen/ResolveMutationThreshold read.
func testMutationConfig() *config.Config {
	return &config.Config{
		Defaults: config.Defaults{MutationThreshold: 80, MutationMaxRounds: 3, TestTimeoutMinutes: 5, MutationTimeoutMinutes: 5},
	}
}

func readTestdata(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestEnsureMutationTargetsTestsGreenPreservesOtherRepos pins the plan's
// design decision 7: a partial refresh must not drop an untouched
// repository's own test-results.json entry.
func TestEnsureMutationTargetsTestsGreenPreservesOtherRepos(t *testing.T) {
	home := t.TempDir()
	ticketDir := filepath.Join(home, "PROJ-1")
	staleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staleDir, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Pre-seed test-results.json: repo "kept" is green with a matching
	// tree_hash (no refresh needed); repo "stale" has no entry at all.
	existing := testResultsArtifact{Repositories: []testResultsRepo{
		{Repo: "kept", Runner: "go", Total: 1, Passed: 1, Failed: 0, TreeHash: sha256Hex("unchanged-diff"), Failures: []testResultsFailure{}},
	}}
	if _, err := handoff.WriteArtifact(ticketDir, "test-results.json", "test-results", existing); err != nil {
		t.Fatal(err)
	}

	se := exectest.New(t).On("go", []string{"test", "-json", "./..."}, exec.Output{Stdout: readTestdata(t, "../verify/unit/testdata/go/pass.jsonl")})

	artifact := &repoArtifact{Repositories: []repoArtifactRepository{
		{Name: "kept"}, {Name: "stale", LocalPath: staleDir},
	}}
	targets := []mutationTarget{
		{repo: repoArtifactRepository{Name: "stale", LocalPath: staleDir}, tool: "gremlins", diff: gitops.DiffResult{Diff: "new-diff-text"}},
	}

	if err := ensureMutationTargetsTestsGreen(context.Background(), testMutationConfig(), ticketDir, artifact, targets, se); err != nil {
		t.Fatalf("ensureMutationTargetsTestsGreen: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(ticketDir, "test-results.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got testResultsArtifact
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Repositories) != 2 {
		t.Fatalf("test-results.json repositories = %d, want 2 (kept preserved, stale refreshed)", len(got.Repositories))
	}
	var kept, stale *testResultsRepo
	for i := range got.Repositories {
		switch got.Repositories[i].Repo {
		case "kept":
			kept = &got.Repositories[i]
		case "stale":
			stale = &got.Repositories[i]
		}
	}
	if kept == nil || kept.TreeHash != sha256Hex("unchanged-diff") {
		t.Errorf("kept entry was dropped or mutated: %+v", kept)
	}
	if stale == nil || stale.TreeHash != sha256Hex("new-diff-text") || stale.Failed != 0 {
		t.Errorf("stale entry not refreshed correctly: %+v", stale)
	}
}

func TestEnsureMutationTargetsTestsGreenRefusesRedSuite(t *testing.T) {
	home := t.TempDir()
	ticketDir := filepath.Join(home, "PROJ-1")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	se := exectest.New(t).On("go", []string{"test", "-json", "./..."}, exec.Output{Stdout: readTestdata(t, "../verify/unit/testdata/go/fail.jsonl")})

	artifact := &repoArtifact{Repositories: []repoArtifactRepository{{Name: "svc", LocalPath: dir}}}
	targets := []mutationTarget{
		{repo: repoArtifactRepository{Name: "svc", LocalPath: dir}, tool: "gremlins", diff: gitops.DiffResult{Diff: "d"}},
	}

	err := ensureMutationTargetsTestsGreen(context.Background(), testMutationConfig(), ticketDir, artifact, targets, se)
	if err == nil {
		t.Fatal("ensureMutationTargetsTestsGreen with a failing suite = nil error, want missing_artifact")
	}
}

// --- CLI-level tests (runRoot, real cobra dispatch) ---

// mutationFakeExecer combines the shared exectest.Scripted double (for
// git) with gremlins' own file-writing behaviour (Tool.Run asks
// gremlins to write its JSON report to whatever path follows
// --output), the same combination unit_test.go's pytestFakeExecer
// needs for a single tool that writes to disk instead of stdout.
type mutationFakeExecer struct {
	*exectest.Scripted
	t       *testing.T
	fixture string // gremlins report fixture to hand back
}

func (f *mutationFakeExecer) Run(ctx context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	if err := exectest.Refused(tool, args); err != nil {
		return exec.Output{}, err // what the real Runner answers (phase 54)
	}
	if tool != "gremlins" {
		return f.Scripted.Run(ctx, tool, args, opts)
	}
	f.t.Helper()
	for i, a := range args {
		if a == "--output" && i+1 < len(args) {
			data, err := os.ReadFile(f.fixture)
			if err != nil {
				f.t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(opts.Dir, args[i+1]), data, 0o600); err != nil {
				f.t.Fatal(err)
			}
			return exec.Output{}, nil
		}
	}
	f.t.Fatalf("gremlins call missing --output: %v", args)
	return exec.Output{}, nil
}

// setUpMutationFixture writes config.json, ticket.json and repo.json
// for a single repository carrying a go.mod (gremlins) plus an
// AGENTS.md (instructionsFileFor), and scripts the git calls
// `mutationTargetsFor`'s one gitops.Diff call needs.
func setUpMutationFixture(t *testing.T, diffFiles, diffText string) (home, ticketDir, repoDir string, se *exectest.Scripted) {
	t.Helper()
	home = t.TempDir()
	localBase := t.TempDir()
	repoDir = repoInstructionsDir(t, localBase, "acme.billing.api")
	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module example.com/acme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeRoutingTestConfig(t, home, localBase, false)
	ticketDir = filepath.Join(home, "PROJ-1")
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", LocalPath: repoDir, InstructionsFile: "AGENTS.md"},
	})

	se = exectest.New(t).
		On("git", []string{"diff", "--name-only", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: diffFiles}).
		On("git", []string{"ls-files", "--others", "--exclude-standard"}, exec.Output{Stdout: ""}).
		On("git", []string{"diff", "--end-of-options", "origin/main...HEAD"}, exec.Output{Stdout: diffText})
	return home, ticketDir, repoDir, se
}

func TestMutationScopeReadOnly(t *testing.T) {
	home, ticketDir, _, se := setUpMutationFixture(t, "main.go\nmain_test.go\n", "diff text")
	useScriptedExecer(t, se)

	stdout, _, exitCode := runRoot(t, "mutation", "scope", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	var env struct {
		Result mutationScopeResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(env.Result.Repositories) != 1 {
		t.Fatalf("repositories = %v", env.Result.Repositories)
	}
	got := env.Result.Repositories[0]
	if !got.Detected || got.Tool != "gremlins" || len(got.Files) != 1 || got.Files[0] != "main.go" {
		t.Errorf("scope result = %+v, want gremlins over [main.go]", got)
	}

	// Read-only: no mutation-reports/ or mutation-results.json.
	if _, err := os.Stat(filepath.Join(ticketDir, "mutation-reports")); !os.IsNotExist(err) {
		t.Error("mutation scope wrote mutation-reports/, want nothing written")
	}
	if _, err := os.Stat(filepath.Join(ticketDir, "mutation-results.json")); !os.IsNotExist(err) {
		t.Error("mutation scope wrote mutation-results.json, want nothing written")
	}
}

func TestMutationRunToolMissing(t *testing.T) {
	// Diff touches only a test file and a doc — no mutable scope for
	// gremlins — so mutationTargetsFor finds nothing, and run refuses
	// without ever touching git's diff-text call or gremlins.
	home, _, _, se := setUpMutationFixture(t, "main_test.go\nREADME.md\n", "diff text")
	useScriptedExecer(t, se)

	stdout, _, exitCode := runRoot(t, "mutation", "run", "PROJ-1", "--home", home)
	if exitCode != 4 {
		t.Fatalf("exitCode = %d, want 4\n%s", exitCode, stdout.String())
	}
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Code != "tool_missing" {
		t.Errorf("error = %+v, want tool_missing", env.Error)
	}
}

func TestMutationRunHappyPath(t *testing.T) {
	home, ticketDir, _, se := setUpMutationFixture(t, "main.go\n", "diff text")
	se.On("go", []string{"test", "-json", "./..."}, exec.Output{Stdout: readTestdata(t, "../verify/unit/testdata/go/pass.jsonl")})
	fx := &mutationFakeExecer{Scripted: se, t: t, fixture: "../mutation/gremlins/testdata/pass.json"}
	orig := newExecer
	newExecer = func() exec.Execer { return fx }
	t.Cleanup(func() { newExecer = orig })

	stdout, _, exitCode := runRoot(t, "mutation", "run", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}

	var env struct {
		Result mutationRunResult `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Result.Round != 1 || len(env.Result.Repositories) != 1 || env.Result.Repositories[0] != "acme.billing.api" {
		t.Errorf("run result = %+v", env.Result)
	}

	reportPath := filepath.Join(ticketDir, "mutation-reports", "round-1", "acme.billing.api.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report not written: %v", err)
	}
	var dto mutationReportDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Tool != "gremlins" || dto.Total != 3 || dto.Killed != 2 {
		t.Errorf("report dto = %+v, want the gremlins pass.json fixture's shape", dto)
	}

	// test-results.json should have been created fresh (tree_hash set,
	// suite tolerated as green since the fixture repo has no real go
	// test to run — wait: unit.Run("go") DOES need a "go test -json"
	// call too, scripted below via the same fake).
	if _, err := os.Stat(filepath.Join(ticketDir, "test-results.json")); err != nil {
		t.Errorf("test-results.json not written: %v", err)
	}
}

func TestMutationSummarizeNoRound(t *testing.T) {
	home, _, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home)
	if exitCode != 3 {
		t.Fatalf("exitCode = %d, want 3\n%s", exitCode, stdout.String())
	}
}

// seedMutationRound writes ticketDir/mutation-reports/round-<N>/<repo>.json
// by hand — the CLI-level summarize tests don't need `mutation run` to
// have actually executed a subprocess, since summarize only ever reads
// this file's normalized JSON shape (mutationReportDTO).
func seedMutationRound(t *testing.T, ticketDir string, round int, repo, tool string, total, killed int, survivors []string) {
	t.Helper()
	var b []byte
	dto := struct {
		Tool      string                `json:"tool"`
		Total     int                   `json:"total"`
		Killed    int                   `json:"killed"`
		Survivors []mutationSurvivorDTO `json:"survivors"`
	}{Tool: tool, Total: total, Killed: killed}
	for i, id := range survivors {
		dto.Survivors = append(dto.Survivors, mutationSurvivorDTO{ID: id, File: "main.go", Line: 10 + i})
	}
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	writeRawJSON(t, mutationRoundDir(ticketDir, round), repo+".json", string(b))
}

func TestMutationSummarizePass(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 2, 2, nil)

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result mutationResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Result.Round != 1 || len(env.Result.Repositories) != 1 || env.Result.Repositories[0].Score != 100 {
		t.Errorf("result = %+v", env.Result)
	}
	if env.Result.Repositories[0].Threshold != 80 {
		t.Errorf("threshold = %d, want 80 (defaults.mutation_threshold)", env.Result.Repositories[0].Threshold)
	}
}

func TestMutationSummarizeBelowThreshold(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 4, 1, []string{"g-0", "g-1", "g-2"})

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home)
	if exitCode != 4 {
		t.Fatalf("exitCode = %d, want 4\n%s", exitCode, stdout.String())
	}
	var env struct {
		NeedsInput *struct {
			Options []string `json:"options"`
		} `json:"needs_input"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Code != "mutation_below_threshold" {
		t.Fatalf("error = %+v, want mutation_below_threshold", env.Error)
	}
	if env.NeedsInput == nil || len(env.NeedsInput.Options) != 2 {
		t.Fatalf("needs_input.options = %v, want [add-tests accept]", env.NeedsInput)
	}

	// mutation-results.json is still written on the needs_input path.
	data, err := os.ReadFile(filepath.Join(ticketDir, "mutation-results.json"))
	if err != nil {
		t.Fatalf("mutation-results.json not written: %v", err)
	}
	var results mutationResultsArtifact
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	if results.Repositories[0].Score != 25 {
		t.Errorf("score = %v, want 25 (1/4 * 100)", results.Repositories[0].Score)
	}
}

func TestMutationSummarizeEquivalentRescoresAndPasses(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	// total=4, killed=3, survivors=[g-0] -> 75, below 80. Marking g-0
	// equivalent drops the denominator to 3 -> 100, above threshold.
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 4, 3, []string{"g-0"})

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home,
		"--equivalent", "g-0", "--reason", "dead branch, unreachable in practice")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result mutationResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	repo := env.Result.Repositories[0]
	if repo.Score != 100 {
		t.Errorf("score = %v, want 100 after equivalent excludes the one survivor", repo.Score)
	}
	if len(repo.Equivalent) != 1 || repo.Equivalent[0].ID != "g-0" {
		t.Errorf("equivalent = %+v", repo.Equivalent)
	}
}

func TestMutationSummarizeEquivalentUnknownID(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 2, 1, []string{"g-0"})

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home,
		"--equivalent", "nonexistent", "--reason", "r")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", exitCode, stdout.String())
	}
}

func TestMutationSummarizeAccept(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 4, 1, []string{"g-0", "g-1", "g-2"})

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home,
		"--accept", "--reason", "acceptable risk for this ticket")
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	var env struct {
		Result mutationResultsArtifact `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Result.AcceptedBelowThreshold == nil || env.Result.AcceptedBelowThreshold.Reason != "acceptable risk for this ticket" {
		t.Errorf("accepted_below_threshold = %+v", env.Result.AcceptedBelowThreshold)
	}
	if env.Result.AcceptedBelowThreshold.By == "" {
		t.Error("accepted_below_threshold.by is empty, want currentUser()")
	}
}

// TestMutationSummarizeEquivalentAmbiguousID pins design decision 11's
// other branch (the plan's own §2 note on this decision, "found in
// zero or more than one repository"): the same survivor id present in
// two repositories' reports in the same round is usage, not a silent
// pick of either.
func TestMutationSummarizeEquivalentAmbiguousID(t *testing.T) {
	home := t.TempDir()
	ticketDir := filepath.Join(home, "PROJ-1")
	writeRoutingTestConfig(t, home, t.TempDir(), false)
	writeTicketFixture(t, ticketDir, "PROJ-1", "s", "d", "feature/PROJ-1")
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{
		{Name: "acme.billing.api", InstructionsFile: "AGENTS.md"},
		{Name: "acme.billing.web", InstructionsFile: "AGENTS.md"},
	})
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 2, 1, []string{"dup-0"})
	seedMutationRound(t, ticketDir, 1, "acme.billing.web", "gremlins", 2, 1, []string{"dup-0"})

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home,
		"--equivalent", "dup-0", "--reason", "r")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage, ambiguous across repositories)\n%s", exitCode, stdout.String())
	}
}

func TestMutationSummarizeMutualExclusion(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 2, 1, []string{"g-0"})

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home,
		"--equivalent", "g-0", "--accept", "--reason", "r")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", exitCode, stdout.String())
	}
}

func TestMutationSummarizeMissingReason(t *testing.T) {
	home, _, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home, "--accept")
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2 (usage)\n%s", exitCode, stdout.String())
	}
}

func TestMutationSummarizeRoundsExhaustionOffersOnlyAccept(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	// writeRoutingTestConfig sets mutation_max_rounds: 3 — seed round 3
	// directly (summarize only cares about the highest round present).
	seedMutationRound(t, ticketDir, 3, "acme.billing.api", "gremlins", 4, 1, []string{"g-0", "g-1", "g-2"})

	stdout, _, exitCode := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home)
	if exitCode != 4 {
		t.Fatalf("exitCode = %d, want 4\n%s", exitCode, stdout.String())
	}
	var env struct {
		NeedsInput *struct {
			Options []string `json:"options"`
		} `json:"needs_input"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.NeedsInput == nil || len(env.NeedsInput.Options) != 1 || env.NeedsInput.Options[0] != "accept" {
		t.Errorf("needs_input.options = %v, want [accept] only after the last round", env.NeedsInput)
	}
}

func TestMutationSummarizeByteStable(t *testing.T) {
	home, ticketDir, _, _ := setUpMutationFixture(t, "main.go\n", "diff text")
	seedMutationRound(t, ticketDir, 1, "acme.billing.api", "gremlins", 2, 2, nil)

	if _, _, ec := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home); ec != 0 {
		t.Fatalf("first summarize: exitCode = %d", ec)
	}
	first, err := os.ReadFile(filepath.Join(ticketDir, "mutation-results.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ec := runRoot(t, "mutation", "summarize", "PROJ-1", "--home", home); ec != 0 {
		t.Fatalf("second summarize: exitCode = %d", ec)
	}
	second, err := os.ReadFile(filepath.Join(ticketDir, "mutation-results.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("mutation-results.json not byte-stable across identical re-runs:\nfirst:  %s\nsecond: %s", first, second)
	}
}
