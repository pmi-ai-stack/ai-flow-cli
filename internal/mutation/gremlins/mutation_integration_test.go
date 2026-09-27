//go:build mutation

package gremlins_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation/gremlins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// TestRealGremlinsRun is this package's own share of the CI gates
// table's "`-tags mutation` ... real tools when on PATH" nightly job —
// a genuine gap found while verifying phase 30 (`.ai/phase-30/
// implementation-log.md`): no file in this module carried a `mutation`
// build tag before this phase, so the documented job had nothing to
// run. Mirrors internal/ops/addons/terraform's own `-tags ops`
// precedent exactly: skip, never fail, when the real binary is absent
// from PATH (a fork PR or a machine without it installed).
//
// The fixture package deliberately has one killable mutant (the `+`
// in Add) and one genuinely uncovered branch (Sub, which no test
// calls) so the real report exercises Parse's own three-way KILLED/
// LIVED/NOT_COVERED split against gremlins' real JSON shape, not a
// hand-shaped fixture file.
func TestRealGremlinsRun(t *testing.T) {
	if _, ok := exec.Which("gremlins"); !ok {
		t.Skip("gremlins not on PATH")
	}

	repoDir := t.TempDir()
	writeFixtureModule(t, repoDir)

	tool := gremlins.Tool{}
	report, err := tool.Run(context.Background(), detect.Repo{Name: "fixture", LocalPath: repoDir},
		mutation.Scope{Repo: "fixture", Files: []string{"calc.go"}}, exec.NewRunner(), 2*time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Total == 0 {
		t.Fatal("report.Total = 0, want at least one mutant found in the fixture module")
	}
	if report.Killed == 0 {
		t.Error("report.Killed = 0, want Add's own test to kill at least the ARITHMETIC_BASE mutant on \"+\"")
	}
}

func writeFixtureModule(t *testing.T, dir string) {
	t.Helper()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module fixture\n\ngo 1.21\n")
	write("calc.go", `package fixture

func Add(a, b int) int {
	return a + b
}

func Sub(a, b int) int {
	return a - b
}
`)
	write("calc_test.go", `package fixture

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Errorf("Add(2, 3) = %d, want 5", got)
	}
}
`)
}
