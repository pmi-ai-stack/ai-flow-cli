package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// pinnedCheckScratch is the directory pinCheckScratch pinned for the
// running test, so every helper that predicts a runner's argv in one
// test agrees on it.
var pinnedCheckScratch string

// pinCheckScratch makes every standards check run in this test use one
// known scratch directory, so a scripted Execer can predict the config,
// policy and report paths the runners put on argv. runEffectiveChecks
// removes the directory after each run, so the seam recreates it.
func pinCheckScratch(t *testing.T) string {
	t.Helper()
	if pinnedCheckScratch != "" {
		return pinnedCheckScratch
	}
	dir := filepath.Join(t.TempDir(), "scratch")
	orig := newCheckScratch
	newCheckScratch = func() (string, error) { return dir, os.MkdirAll(dir, 0o700) }
	pinnedCheckScratch = dir
	t.Cleanup(func() {
		newCheckScratch = orig
		pinnedCheckScratch = ""
	})
	return dir
}

// TestNewCheckScratchIsPrivateAndUnique: the production seam makes a
// fresh owner-only directory per call, never a shared fixed path.
func TestNewCheckScratchIsPrivateAndUnique(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a, err := newCheckScratch()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newCheckScratch()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("two calls returned the same directory %s", a)
	}
	for _, dir := range []string{a, b} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, want a 0700 directory", dir, info.Mode())
		}
		if filepath.Dir(dir) != os.TempDir() || !strings.HasPrefix(filepath.Base(dir), "ai-flow-check-") {
			t.Errorf("%s: want ai-flow-check-* directly under %s", dir, os.TempDir())
		}
	}
}

// recordCheckScratch wraps the seam so a test learns which directory a
// run used, then can assert it is gone.
func recordCheckScratch(t *testing.T) *[]string {
	t.Helper()
	var made []string
	orig := newCheckScratch
	newCheckScratch = func() (string, error) {
		dir, err := orig()
		made = append(made, dir)
		return dir, err
	}
	t.Cleanup(func() { newCheckScratch = orig })
	return &made
}

func TestStandardsCheckRemovesItsScratch(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-60")
	made := recordCheckScratch(t)

	// The scripted double cannot predict a random directory, so it
	// answers any argv for the three tools the embedded pack runs.
	x := anyArgvExecer{}
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	runChecks(t, 0, "standards", "check", "PROJ-60", "--home", home)
	assertScratchRemoved(t, *made)
}

func TestStandardsCheckRemovesScratchOnRunnerError(t *testing.T) {
	home, _ := standardsTicket(t, "PROJ-61")
	made := recordCheckScratch(t)

	x := exectest.New(t).OnFail("gofmt", []string{"-l", "."}, errors.New("boom"))
	orig := newExecer
	newExecer = func() exec.Execer { return x }
	t.Cleanup(func() { newExecer = orig })

	if _, _, ec := runRoot(t, "standards", "check", "PROJ-61", "--home", home); ec == 0 {
		t.Fatal("exit 0, want the runner's error")
	}
	assertScratchRemoved(t, *made)
}

func assertScratchRemoved(t *testing.T, made []string) {
	t.Helper()
	if len(made) != 1 {
		t.Fatalf("scratch directories made = %v, want exactly one per run", made)
	}
	if _, err := os.Stat(made[0]); !os.IsNotExist(err) {
		t.Errorf("scratch %s still exists after the run (stat err %v)", made[0], err)
	}
}

// anyArgvExecer answers every allowlisted call with empty output, after
// the same allowlist check exectest.Scripted makes.
type anyArgvExecer struct{}

func (a anyArgvExecer) Run(_ context.Context, tool string, args []string, _ exec.RunOpts) (exec.Output, error) {
	if err := exectest.Refused(tool, args); err != nil {
		return exec.Output{}, err
	}
	if tool == "golangci-lint" {
		return exec.Output{Stdout: `{"Issues":[]}`}, nil
	}
	return exec.Output{}, nil
}
