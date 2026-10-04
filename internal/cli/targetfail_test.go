package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func tgt(repo, path string) ops.Target { return ops.Target{Addon: "kubectl", Repo: repo, Path: path} }

func TestTargetFailuresNoneIsNil(t *testing.T) {
	var tf targetFailures
	if err := tf.err(3); err != nil {
		t.Errorf("err() with no failures = %v, want nil", err)
	}
}

func TestTargetFailuresSingleTargetKeepsTheOriginalError(t *testing.T) {
	var tf targetFailures
	orig := &envelope.Error{Code: "missing_artifact", Message: "no base checkout", Details: []string{"x"}}
	tf.add(tgt("a", "p"), orig)
	if got := tf.err(1); !errors.Is(got, orig) {
		t.Errorf("single target: got %v, want the original error untouched", got)
	}
}

func TestTargetFailuresAggregateInTargetOrder(t *testing.T) {
	var tf targetFailures
	tf.add(tgt("a", "x"), &envelope.Error{Code: "missing_artifact", Message: "no checkout"})
	tf.add(tgt("b", "y"), &envelope.Error{Code: "tool_failed", Message: "terraform: boom"})

	err := tf.err(5)
	var e *envelope.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %T, want *envelope.Error", err)
	}
	if e.Code != "missing_artifact" {
		t.Errorf("code = %q, want the first failure's code", e.Code)
	}
	if e.Message != "2 of 5 targets failed" {
		t.Errorf("message = %q", e.Message)
	}
	want := []string{"a:x: missing_artifact: no checkout", "b:y: tool_failed: terraform: boom"}
	if strings.Join(e.Details, "|") != strings.Join(want, "|") {
		t.Errorf("details = %q, want %q", e.Details, want)
	}
}

func TestTargetFailuresPlainErrorsAreInternal(t *testing.T) {
	var tf targetFailures
	tf.add(tgt("a", "x"), errors.New("disk on fire"))
	tf.add(tgt("b", "y"), errors.New("again"))
	var e *envelope.Error
	if !errors.As(tf.err(2), &e) || e.Code != "internal" {
		t.Fatalf("got %v, want an internal envelope error", tf.err(2))
	}
}

func TestTargetFailuresFirstExposesTheFirstError(t *testing.T) {
	var tf targetFailures
	first := &envelope.Error{Code: "live_credentials_missing", Message: "m"}
	tf.add(tgt("a", "x"), first)
	tf.add(tgt("b", "y"), errors.New("other"))
	if !errors.Is(tf.first(), first) {
		t.Errorf("first() = %v, want the first added error", tf.first())
	}
}

// multiTargetTicket builds a ticket whose repo.json names only
// acme.infra, with one good kubectl target (when withGood) and the given repositories
// repo.json does not know — each of those fails deterministically with
// missing_artifact, with no external tool involved.
func multiTargetTicket(t *testing.T, key string, withGood bool, ghosts ...string) (home string) {
	t.Helper()
	home = t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: %d", ec)
	}
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "k8s", "a"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFileTest(t, filepath.Join(repoDir, "k8s", "a", "deploy.yaml"), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n")
	ticketDir := filepath.Join(home, key)
	writeRepoFixture(t, ticketDir, "Team Beta", []repoArtifactRepository{{Name: "acme.infra", LocalPath: repoDir, InstructionsFile: "AGENTS.md"}})

	yes := true
	var targets []ops.Target
	if withGood {
		targets = append(targets, ops.Target{Addon: "kubectl", Repo: "acme.infra", Path: "k8s/a", Scope: ops.ScopeDiff, Changed: &yes})
	}
	for _, g := range ghosts {
		targets = append(targets, ops.Target{Addon: "kubectl", Repo: g, Path: "k8s/z", Scope: ops.ScopeDiff, Changed: &yes})
	}
	writeOpsTargetsFixture(t, home, key, targets)
	return home
}

type failEnvelope struct {
	OK    bool `json:"ok"`
	Error *struct {
		Code    string   `json:"code"`
		Message string   `json:"message"`
		Details []string `json:"details"`
	} `json:"error"`
}

func runOpsFailing(t *testing.T, wantExit int, args ...string) failEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, args...)
	if ec != wantExit {
		t.Fatalf("%v: exitCode = %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	var env failEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("%v: stdout is not valid JSON: %v\n%s", args, err, stdout.String())
	}
	return env
}

func TestOpsRenderReportsEveryFailingTarget(t *testing.T) {
	home := multiTargetTicket(t, "PROJ-1", true, "ghost1", "ghost2")
	env := runOpsFailing(t, 3, "ops", "render", "PROJ-1", "--ref", "branch", "--home", home)
	if env.Error == nil || env.Error.Code != "missing_artifact" {
		t.Fatalf("error = %+v, want missing_artifact", env.Error)
	}
	if env.Error.Message != "2 of 3 targets failed" {
		t.Errorf("message = %q, want \"2 of 3 targets failed\" (the good target is not a failure)", env.Error.Message)
	}
	if len(env.Error.Details) != 2 || !strings.HasPrefix(env.Error.Details[0], "ghost1:k8s/z: missing_artifact: ") || !strings.HasPrefix(env.Error.Details[1], "ghost2:k8s/z: missing_artifact: ") {
		t.Errorf("details = %q, want one line per failing target in target order", env.Error.Details)
	}
}

func TestOpsRenderSingleTargetFailureIsUnchanged(t *testing.T) {
	home := multiTargetTicket(t, "PROJ-1", true, "ghost1")
	env := runOpsFailing(t, 3, "ops", "render", "PROJ-1", "--ref", "branch", "--target", "ghost1:k8s/z", "--home", home)
	if env.Error == nil || len(env.Error.Details) != 0 || !strings.Contains(env.Error.Message, "repo.json names no repository") {
		t.Fatalf("error = %+v, want the original single-target message with no details", env.Error)
	}
}

func TestOpsRenderAllGoodStillSucceeds(t *testing.T) {
	home := multiTargetTicket(t, "PROJ-1", true)
	stdout, _, ec := runRoot(t, "ops", "render", "PROJ-1", "--ref", "branch", "--home", home)
	if ec != 0 {
		t.Fatalf("exitCode = %d\n%s", ec, stdout.String())
	}
}

func TestOpsDriftReportsEveryFailingTargetAndWritesNoArtifact(t *testing.T) {
	home := multiTargetTicket(t, "PROJ-1", false, "ghost1", "ghost2")
	env := runOpsFailing(t, 3, "ops", "drift", "PROJ-1", "--home", home)
	if env.Error == nil || env.Error.Message != "2 of 2 targets failed" || len(env.Error.Details) != 2 {
		t.Fatalf("error = %+v, want 2 of 2 targets failed with two details", env.Error)
	}
	if _, err := os.Stat(filepath.Join(home, "PROJ-1", "render-drift-results.json")); !os.IsNotExist(err) {
		t.Errorf("render-drift-results.json was written despite failing targets (stat err = %v)", err)
	}
}

func TestOpsDryRunReportsEveryFailingTargetAndWritesNoArtifact(t *testing.T) {
	home := multiTargetTicket(t, "PROJ-1", false, "ghost1", "ghost2")
	if _, err := handoff.WriteArtifact(filepath.Join(home, "PROJ-1"), "render-drift-results.json", "render-drift-results", renderDriftArtifact{Targets: []opsDriftResultTarget{}}); err != nil {
		t.Fatal(err)
	}
	env := runOpsFailing(t, 3, "ops", "dry-run", "PROJ-1", "--home", home)
	if env.Error == nil || env.Error.Message != "2 of 2 targets failed" || len(env.Error.Details) != 2 {
		t.Fatalf("error = %+v, want 2 of 2 targets failed with two details", env.Error)
	}
	if _, err := os.Stat(filepath.Join(home, "PROJ-1", "dryrun-results.json")); !os.IsNotExist(err) {
		t.Errorf("dryrun-results.json was written despite failing targets (stat err = %v)", err)
	}
}
