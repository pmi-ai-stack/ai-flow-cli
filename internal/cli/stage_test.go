package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeFixture copies internal/schema/testdata/<artifact>/valid.json —
// the same fixtures internal/schema's own tests validate against — into
// home/key/filename, creating the ticket directory if needed. These
// fixtures double as this package's "stub artifacts" for driving a
// stage through `stage complete`'s existence+schema check.
func writeFixture(t *testing.T, home, key, artifact, filename string) {
	t.Helper()
	src := filepath.Join("..", "schema", "testdata", artifact, "valid.json")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", src, err)
	}
	dir := filepath.Join(home, key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o600); err != nil {
		t.Fatalf("writing %s: %v", filename, err)
	}
}

func writeMarkdown(t *testing.T, home, key, filename string) {
	t.Helper()
	dir := filepath.Join(home, key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("stub\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", filename, err)
	}
}

type stageEnvelope struct {
	OK     bool `json:"ok"`
	Result struct {
		Stage string `json:"stage"`
		State string `json:"state"`
	} `json:"result"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeStage(t *testing.T, raw []byte) stageEnvelope {
	t.Helper()
	var env stageEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, raw)
	}
	return env
}

// runStage runs `ai-flow stage <verb> KEY STAGE [flags...]` and fails
// the test unless the exit code matches wantExit, returning the
// decoded envelope either way.
func runStage(t *testing.T, home string, wantExit int, args ...string) stageEnvelope {
	t.Helper()
	full := append(append([]string{"stage"}, args...), "--home", home)
	stdout, _, exitCode := runRoot(t, full...)
	if exitCode != wantExit {
		t.Fatalf("stage %v: exitCode = %d, want %d\n%s", args, exitCode, wantExit, stdout.String())
	}
	return decodeStage(t, stdout.Bytes())
}

// TestStageFullSixteenStageWalkthrough drives one ticket through every
// stage from fetch to ship, using the phase-2 schema fixtures as stub
// artifacts — this is the CLI-level version of the plan's "Done when"
// walkthrough, already proven by hand during implementation (see the
// log's manual smoke test).
func TestStageFullSixteenStageWalkthrough(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"

	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	type step struct {
		stage    string
		artifact string // schema artifact name, or "" for a markdown stub
		filename string
	}
	steps := []step{
		{"fetch", "ticket", "ticket.json"},
		{"locate", "repo", "repo.json"},
		{"plan", "", "plan.md"},
		{"implement", "test-results", "test-results.json"},
		{"render-drift", "render-drift-results", "render-drift-results.json"},
		{"checks", "checks-results", "checks-results.json"},
		{"dry-run", "dryrun-results", "dryrun-results.json"},
		{"impact", "impact-results", "impact-results.json"},
		{"rollback", "rollback-results", "rollback-results.json"},
		{"infra-test", "infra-test-results", "infra-test-results.json"},
		{"mutation", "mutation-results", "mutation-results.json"},
		{"visual-test", "visual-test-results", "visual-test-results.json"},
		{"e2e", "e2e-results", "e2e-results.json"},
		{"review", "", "review.md"},
		{"output", "", "output.md"},
	}

	for _, s := range steps {
		runStage(t, home, 0, "start", key, s.stage)
		if s.artifact != "" {
			writeFixture(t, home, key, s.artifact, s.filename)
		} else {
			writeMarkdown(t, home, key, s.filename)
		}
		env := runStage(t, home, 0, "complete", key, s.stage)

		switch s.stage {
		case "plan":
			if env.Result.State != "awaiting_approval" {
				t.Fatalf("stage %s: state = %s, want awaiting_approval (plan always gates)", s.stage, env.Result.State)
			}
			runStage(t, home, 0, "approve", key, s.stage)
		case "impact":
			// Fixture score (40) equals the freshly-inited template's
			// risk_gate (40): the gate must fire.
			if env.Result.State != "awaiting_approval" {
				t.Fatalf("stage %s: state = %s, want awaiting_approval (score == risk_gate must fire)", s.stage, env.Result.State)
			}
			runStage(t, home, 0, "approve", key, s.stage)
		default:
			if env.Result.State != "pass" {
				t.Fatalf("stage %s: state = %s, want pass", s.stage, env.Result.State)
			}
		}
	}

	// ship: pre-gate — start lands in awaiting_approval directly.
	env := runStage(t, home, 0, "start", key, "ship")
	if env.Result.State != "awaiting_approval" {
		t.Fatalf("ship start: state = %s, want awaiting_approval", env.Result.State)
	}
	env = runStage(t, home, 0, "approve", key, "ship")
	if env.Result.State != "running" {
		t.Fatalf("ship approve: state = %s, want running", env.Result.State)
	}
	writeFixture(t, home, key, "ship-results", "ship-results.json")
	env = runStage(t, home, 0, "complete", key, "ship")
	if env.Result.State != "pass" {
		t.Fatalf("ship complete: state = %s, want pass", env.Result.State)
	}

	stdout, _, exitCode := runRoot(t, "status", key, "--home", home)
	if exitCode != 0 {
		t.Fatalf("status: exitCode = %d", exitCode)
	}
	final := decodeStatus(t, stdout.Bytes())
	if final.Result.NextStage != nil {
		t.Errorf("next_stage = %v, want nil once every stage is done", *final.Result.NextStage)
	}

	// active.json and events.jsonl exist and name this ticket.
	activeData, err := os.ReadFile(filepath.Join(home, "active.json"))
	if err != nil {
		t.Fatalf("reading active.json: %v", err)
	}
	var active struct {
		Key   string `json:"key"`
		Stage string `json:"stage"`
	}
	if err := json.Unmarshal(activeData, &active); err != nil {
		t.Fatalf("decoding active.json: %v", err)
	}
	if active.Key != key || active.Stage != "ship" {
		t.Errorf("active.json = %+v, want key=%s stage=ship", active, key)
	}
	if _, err := os.Stat(filepath.Join(home, key, "events.jsonl")); err != nil {
		t.Errorf("events.jsonl was not written: %v", err)
	}
}

func TestStageCompleteBeforeStartIsRefused(t *testing.T) {
	home := t.TempDir()
	env := runStage(t, home, 3, "complete", "PROJ-1", "fetch")
	if env.Error == nil || env.Error.Code != "stage_not_allowed" {
		t.Errorf("error = %+v, want stage_not_allowed", env.Error)
	}
}

func TestStageApprovePendingIsRefused(t *testing.T) {
	home := t.TempDir()
	env := runStage(t, home, 3, "approve", "PROJ-1", "plan")
	if env.Error == nil || env.Error.Code != "stage_not_allowed" {
		t.Errorf("error = %+v, want stage_not_allowed", env.Error)
	}
}

func TestStageUnknownStageIsUsageError(t *testing.T) {
	home := t.TempDir()
	env := runStage(t, home, 2, "start", "PROJ-1", "no-such-stage")
	if env.Error == nil || env.Error.Code != "usage" {
		t.Errorf("error = %+v, want usage", env.Error)
	}
}

func TestStageSkipRequiresReason(t *testing.T) {
	home := t.TempDir()
	env := runStage(t, home, 2, "skip", "PROJ-1", "locate")
	if env.Error == nil || env.Error.Code != "usage" {
		t.Errorf("error = %+v, want usage", env.Error)
	}
}

func TestStageSkipWithReasonSucceeds(t *testing.T) {
	home := t.TempDir()
	env := runStage(t, home, 0, "skip", "PROJ-1", "locate", "--reason", "not applicable")
	if env.Result.State != "skipped" {
		t.Errorf("state = %s, want skipped", env.Result.State)
	}
}

func TestStageCompleteMissingPrimaryArtifact(t *testing.T) {
	home := t.TempDir()
	runStage(t, home, 0, "start", "PROJ-1", "fetch")
	env := runStage(t, home, 3, "complete", "PROJ-1", "fetch")
	if env.Error == nil || env.Error.Code != "missing_artifact" {
		t.Errorf("error = %+v, want missing_artifact", env.Error)
	}
}

// TestStageChecksRefusedOnOpenMustFinding proves must_findings_open
// fires on `stage complete KEY checks` and clears once the finding is
// no longer open.
func TestStageChecksRefusedOnOpenMustFinding(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	runStage(t, home, 0, "start", key, "checks")

	dir := filepath.Join(home, key)
	openFinding := []byte(`{"findings":[{"check_id":"x","layer":"org","kind":"lint","stage":"checks","severity":"must","tool":"t","message":"m","status":"open"}]}`)
	if err := os.WriteFile(filepath.Join(dir, "checks-results.json"), openFinding, 0o600); err != nil {
		t.Fatalf("writing checks-results.json: %v", err)
	}
	env := runStage(t, home, 3, "complete", key, "checks")
	if env.Error == nil || env.Error.Code != "must_findings_open" {
		t.Errorf("error = %+v, want must_findings_open", env.Error)
	}

	writeFixture(t, home, key, "checks-results", "checks-results.json") // no open must findings
	env = runStage(t, home, 0, "complete", key, "checks")
	if env.Result.State != "pass" {
		t.Errorf("state = %s, want pass once the must finding is resolved", env.Result.State)
	}
}

// TestStageShipApproveRefusedOnOutputNotPassed and
// TestStageShipApproveRefusedOnOpenMustFinding exercise `stage approve
// KEY ship`'s two extra preconditions directly, without running the
// full pipeline first.
func TestStageShipApproveRefusedOnOutputNotPassed(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	runStage(t, home, 0, "start", key, "ship")
	env := runStage(t, home, 3, "approve", key, "ship")
	if env.Error == nil || env.Error.Code != "output_not_passed" {
		t.Errorf("error = %+v, want output_not_passed", env.Error)
	}
}

func TestStageShipApproveRefusedOnOpenMustFinding(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	dir := filepath.Join(home, key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	statusJSON := []byte(`{"key":"PROJ-1","stages":[` +
		`{"stage":"fetch","state":"pass","approval":null},` +
		`{"stage":"locate","state":"pass","approval":null},` +
		`{"stage":"plan","state":"approved","approval":{"by":"a","at":"2026-01-01T00:00:00Z","gate":"post"}},` +
		`{"stage":"implement","state":"pass","approval":null},` +
		`{"stage":"render-drift","state":"pass","approval":null},` +
		`{"stage":"checks","state":"pass","approval":null},` +
		`{"stage":"dry-run","state":"pass","approval":null},` +
		`{"stage":"impact","state":"approved","approval":{"by":"a","at":"2026-01-01T00:00:00Z","gate":"post"}},` +
		`{"stage":"rollback","state":"pass","approval":null},` +
		`{"stage":"infra-test","state":"pass","approval":null},` +
		`{"stage":"mutation","state":"pass","approval":null},` +
		`{"stage":"visual-test","state":"pass","approval":null},` +
		`{"stage":"e2e","state":"pass","approval":null},` +
		`{"stage":"review","state":"pass","approval":null},` +
		`{"stage":"output","state":"pass","approval":null},` +
		`{"stage":"ship","state":"awaiting_approval","approval":null}` +
		`]}`)
	if err := os.WriteFile(filepath.Join(dir, "status.json"), statusJSON, 0o600); err != nil {
		t.Fatalf("writing status.json: %v", err)
	}
	openFinding := []byte(`{"findings":[{"check_id":"x","layer":"org","kind":"lint","stage":"checks","severity":"must","tool":"t","message":"m","status":"open"}]}`)
	if err := os.WriteFile(filepath.Join(dir, "checks-results.json"), openFinding, 0o600); err != nil {
		t.Fatalf("writing checks-results.json: %v", err)
	}

	env := runStage(t, home, 3, "approve", key, "ship")
	if env.Error == nil || env.Error.Code != "must_findings_open" {
		t.Errorf("error = %+v, want must_findings_open", env.Error)
	}
}

func TestStageResetRefusedFromNonTerminal(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	runStage(t, home, 0, "start", key, "fetch") // now running, not terminal
	env := runStage(t, home, 3, "reset", key, "fetch")
	if env.Error == nil || env.Error.Code != "stage_not_allowed" {
		t.Errorf("error = %+v, want stage_not_allowed", env.Error)
	}
}

// TestStageResetDoesNotTouchRenderPlansBeforeImplement proves the
// render/plans cleanup is scoped to "implement onward", not every
// reset.
func TestStageResetDoesNotTouchRenderPlansBeforeImplement(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	runStage(t, home, 0, "start", key, "fetch")
	writeFixture(t, home, key, "ticket", "ticket.json")
	runStage(t, home, 0, "complete", key, "fetch")

	dir := filepath.Join(home, key)
	if err := os.MkdirAll(filepath.Join(dir, "render"), 0o700); err != nil {
		t.Fatalf("MkdirAll render: %v", err)
	}

	env := runStage(t, home, 0, "reset", key, "fetch")
	if env.Result.State != "pending" {
		t.Errorf("state = %s, want pending", env.Result.State)
	}
	if _, err := os.Stat(filepath.Join(dir, "render")); err != nil {
		t.Errorf("render/ should NOT be removed when resetting fetch (before implement): %v", err)
	}
}

// TestStageResetCascadesAndRemovesRenderPlans proves reset's cascade
// (later stages only, approvals on earlier stages untouched) and its
// "implement onward" render/plans cleanup from the CLI layer — the CLI
// equivalent of the manual smoke test recorded in the implementation
// log.
func TestStageResetCascadesAndRemovesRenderPlans(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}

	runStage(t, home, 0, "start", key, "fetch")
	writeFixture(t, home, key, "ticket", "ticket.json")
	runStage(t, home, 0, "complete", key, "fetch")

	runStage(t, home, 0, "start", key, "locate")
	writeFixture(t, home, key, "repo", "repo.json")
	runStage(t, home, 0, "complete", key, "locate")

	runStage(t, home, 0, "start", key, "plan")
	writeMarkdown(t, home, key, "plan.md")
	runStage(t, home, 0, "complete", key, "plan")
	runStage(t, home, 0, "approve", key, "plan")

	runStage(t, home, 0, "start", key, "implement")
	writeFixture(t, home, key, "test-results", "test-results.json")
	runStage(t, home, 0, "complete", key, "implement")

	runStage(t, home, 0, "start", key, "render-drift")
	writeFixture(t, home, key, "render-drift-results", "render-drift-results.json")
	runStage(t, home, 0, "complete", key, "render-drift")

	dir := filepath.Join(home, key)
	if err := os.MkdirAll(filepath.Join(dir, "render", "fake"), 0o700); err != nil {
		t.Fatalf("MkdirAll render: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "plans"), 0o700); err != nil {
		t.Fatalf("MkdirAll plans: %v", err)
	}

	env := runStage(t, home, 0, "reset", key, "implement")
	if env.Result.State != "pending" {
		t.Errorf("state = %s, want pending", env.Result.State)
	}
	if _, err := os.Stat(filepath.Join(dir, "render")); !os.IsNotExist(err) {
		t.Error("render/ should have been removed on reset of implement")
	}
	if _, err := os.Stat(filepath.Join(dir, "plans")); !os.IsNotExist(err) {
		t.Error("plans/ should have been removed on reset of implement")
	}

	stdout, _, exitCode := runRoot(t, "status", key, "--home", home)
	if exitCode != 0 {
		t.Fatalf("status: exitCode = %d", exitCode)
	}
	st := decodeStatus(t, stdout.Bytes())
	if st.Result.NextStage == nil || *st.Result.NextStage != "implement" {
		t.Errorf("next_stage = %v, want implement", st.Result.NextStage)
	}

	var full struct {
		Result struct {
			Stages []struct {
				Stage    string `json:"stage"`
				State    string `json:"state"`
				Approval *struct {
					By string `json:"by"`
				} `json:"approval"`
			} `json:"stages"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &full); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, s := range full.Result.Stages {
		switch s.Stage {
		case "fetch", "locate":
			if s.State != "pass" {
				t.Errorf("stage %s: state = %s, want pass (untouched, before implement)", s.Stage, s.State)
			}
		case "plan":
			if s.State != "approved" || s.Approval == nil {
				t.Errorf("stage plan: state = %s, approval = %v, want approved with its approval kept", s.State, s.Approval)
			}
		case "render-drift":
			if s.State != "pending" {
				t.Errorf("stage render-drift: state = %s, want pending (cascaded)", s.State)
			}
		}
	}
}

func TestStageShouldRun(t *testing.T) {
	home := t.TempDir()
	var full struct {
		Result struct {
			Stage string `json:"stage"`
			Rule  string `json:"rule"`
			Run   bool   `json:"run"`
		} `json:"result"`
	}
	stdout, _, exitCode := runRoot(t, "stage", "should-run", "PROJ-1", "render-drift", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d\n%s", exitCode, stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &full); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !full.Result.Run {
		t.Error("run = false, want true")
	}
	if full.Result.Rule != "no_changed_target" {
		t.Errorf("rule = %q, want no_changed_target", full.Result.Rule)
	}
	// should-run must never write status.json — it is read-only.
	if _, err := os.Stat(filepath.Join(home, "PROJ-1", "status.json")); !os.IsNotExist(err) {
		t.Error("stage should-run wrote status.json, but it must never mutate anything")
	}
}
