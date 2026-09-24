package githubactions_test

import (
	"context"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
)

// TestDriftIsNotApplicableAndUncalledInProduction proves the method
// itself is safe to call directly (satisfying the interface) even
// though internal/cli/ops_drift.go never reaches it in production
// (Capabilities().Drift is false — see addon_test.go).
func TestDriftIsNotApplicableAndUncalledInProduction(t *testing.T) {
	x := exectest.New(t)
	a := githubactions.New(x, devConfig())
	report, err := a.Drift(context.Background(), ops.Target{}, ops.Env{}, &ops.Rendered{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if report.Drift != ops.DriftNotApplicable {
		t.Errorf("Drift = %q, want not_applicable", report.Drift)
	}
	if len(x.Calls) != 0 {
		t.Errorf("Calls = %v, want none — Drift never runs a subprocess", x.Calls)
	}
}
