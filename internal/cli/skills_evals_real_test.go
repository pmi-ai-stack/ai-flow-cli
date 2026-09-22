//go:build eval

package cli

import (
	"os"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// TestSkillsEvals_RealHeadlessRun runs `skills evals` for real against
// this repository's own testdata/skills/valid/skills/develop — the
// only fixture-complete skill under testdata whose evals.json names a
// ticket key in every scenario's query (design decision 3's own
// substring matching, and Workspace's fixture staging, both need one).
// Gated behind -tags eval, exactly like -tags integration/mutation/ops
// (07-determinism.md's "real tools only behind build tags"): neither a
// `claude` binary nor ANTHROPIC_API_KEY is guaranteed present in every
// environment this test suite runs in — see the phase 11 plan's §1.3
// for why this half is not, and cannot be, verified in this sandbox.
func TestSkillsEvals_RealHeadlessRun(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	if _, found := exec.Which("claude"); !found {
		t.Skip("claude is not on PATH")
	}

	home := t.TempDir()
	out := t.TempDir()
	stdout, stderr, exitCode := runRoot(t, "skills", "evals", "../../testdata/skills/valid", "--skill", "develop", "--out", out, "--home", home)

	// A real model may or may not clear the pass threshold — that is
	// not this test's own assertion (see §1.3: this environment cannot
	// verify a real score). What this test proves is that the wiring
	// itself is reachable end to end: either ok:true, or a failing
	// invalid_result envelope with real grader/order detail, never an
	// internal/usage-shaped failure that would mean the harness itself
	// is broken.
	if exitCode != 0 && exitCode != 6 {
		t.Fatalf("exitCode = %d, want 0 or 6\nstdout:\n%s\nstderr:\n%s", exitCode, stdout.String(), stderr.String())
	}
	t.Logf("skills evals (real): exit=%d\n%s", exitCode, stdout.String())
}
