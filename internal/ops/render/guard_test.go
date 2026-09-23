package render_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/guard"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/testdata/fixtureaddon"
)

// TestGuardDeniesWritesUnderARealRenderCachePath is the plan's own
// growing-surfaces line: "the guard's ~/.ai-flow/cache/** row is
// proven end to end here." internal/agenthost/guard's row3_cache has
// been unit-tested against synthetic paths since phase 9; this is the
// first test that feeds it a path this codebase's own code actually
// produced, via a real render.Render call.
func TestGuardDeniesWritesUnderARealRenderCachePath(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	addon := fixtureaddon.New("fixture", "infra")
	res, err := render.Render(context.Background(), render.Options{
		Addon:     addon,
		Target:    ops.Target{Addon: "fixture", Repo: "acme.infra", Path: "infra"},
		Ref:       ops.RefBranch,
		SourceDir: src,
		Home:      home,
		Key:       "PROJ-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	realPath := filepath.Join(render.CachePath(home, res.Hash), "a.txt")
	decision := guard.Evaluate(guard.Input{
		Event: agenthost.HookEvent{Event: agenthost.EventWrite, FilePath: realPath},
		Home:  home,
		Idle:  8 * time.Hour,
		Now:   time.Now(),
	})
	if decision.Allow {
		t.Fatalf("guard.Evaluate allowed a write to the real rendering cache at %s, want denied (row3_cache)", realPath)
	}
	if decision.Reason != "row3_cache" {
		t.Errorf("decision.Reason = %q, want row3_cache", decision.Reason)
	}
}
