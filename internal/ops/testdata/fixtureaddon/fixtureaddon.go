// Package fixtureaddon is a trivial ops.Addon: every capability
// returns a zero-value success with no error, and Detect returns one
// Target per configured path, claiming every changed file under it.
// It exists so phases 18-20 and 23 can prove the tool-independent OPS
// contract (registry, detection, environment binding, rendering,
// drift, dry-run, impact) without a real tool — the plan's own
// "Sequencing risks" row names this explicitly. It lives under
// testdata/ per the plan's own words: go vet ./... and golangci-lint
// run ./... do not discover it via wildcard expansion, but it is still
// compiled and checked whenever a test imports it directly, which
// every test that uses it does.
package fixtureaddon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// Addon is the fixture. Zero value is usable directly (New just sets
// Name for a clearer test failure message); every field is exported so
// a test can shape its behavior precisely.
type Addon struct {
	AddonName string
	Caps      ops.Capabilities
	// Paths is every target path (relative to a repo's own checkout
	// root) this fixture claims, one Target per entry.
	Paths []string
	// SensitiveFiles names files (by base name) whose whole content
	// Render marks as a redact.MarkedValue — phase 19's own way of
	// proving the real/redacted split without a real tool.
	SensitiveFiles []string
	// RenderCalls counts real Render invocations — phase 19's proof
	// that a cache hit never calls it a second time for the same
	// content.
	RenderCalls int
}

var _ ops.Addon = (*Addon)(nil)

// New returns a fixture addon named name, claiming paths, with every
// capability present.
func New(name string, paths ...string) *Addon {
	return &Addon{
		AddonName: name,
		Caps: ops.Capabilities{
			Render: true, Validate: true, Drift: true, DryRun: true,
			Rollback: true, UnitTest: true, E2ETest: true,
		},
		Paths: paths,
	}
}

func (a *Addon) Name() string                   { return a.AddonName }
func (a *Addon) Capabilities() ops.Capabilities { return a.Caps }

// Detect returns one Target per a.Paths, each claiming every entry of
// changed that lies under that path (or equals it exactly). changed
// nil (scope "repo") yields targets with an empty Files.
func (a *Addon) Detect(_ detect.Repo, changed []string) ([]ops.Target, error) {
	targets := make([]ops.Target, 0, len(a.Paths))
	for _, p := range a.Paths {
		var files []string
		prefix := p + "/"
		for _, f := range changed {
			if f == p || strings.HasPrefix(f, prefix) {
				files = append(files, f)
			}
		}
		targets = append(targets, ops.Target{Addon: a.AddonName, Path: p, Files: files})
	}
	return targets, nil
}

// Render copies every regular file directly under t.Path (the
// call-scoped, resolved-to-absolute source directory internal/ops/
// render passes — see the phase-18 doc comment on Addon.Detect and
// the phase-19 plan's own design decision on this) into out, and
// marks the whole content of any file named in a.SensitiveFiles —
// enough to exercise the real/redacted split, memoization and
// cross-ticket cache sharing without a real tool. Mimics the
// capability matrix's own "kubectl: copy" the most closely of the six
// real addons.
func (a *Addon) Render(_ context.Context, t ops.Target, _ ops.Ref, out string) (*ops.Rendered, error) {
	a.RenderCalls++

	entries, err := os.ReadDir(t.Path)
	if err != nil {
		return nil, fmt.Errorf("fixtureaddon: read %s: %w", t.Path, err)
	}
	var marked []redact.MarkedValue
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(t.Path, e.Name())) //nolint:gosec // G304: t.Path is a test-fixture source directory, not attacker-controlled
		if err != nil {
			return nil, fmt.Errorf("fixtureaddon: read %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(out, e.Name()), data, 0o600); err != nil {
			return nil, fmt.Errorf("fixtureaddon: write %s: %w", e.Name(), err)
		}
		if slices.Contains(a.SensitiveFiles, e.Name()) {
			marked = append(marked, redact.MarkedValue{Pointer: e.Name(), Value: string(data)})
		}
	}
	return &ops.Rendered{Dir: out, Marked: marked}, nil
}

func (a *Addon) Validate(_ context.Context, _ *ops.Rendered) ([]ops.Finding, error) {
	return nil, nil
}

func (a *Addon) Drift(_ context.Context, _ ops.Target, _ ops.Env, _ *ops.Rendered) (*ops.DriftReport, error) {
	return &ops.DriftReport{Drift: ops.DriftNone}, nil
}

func (a *Addon) DryRun(_ context.Context, _ ops.Target, _ ops.Env, _ *ops.Rendered) (*ops.Changeset, error) {
	return &ops.Changeset{}, nil
}

func (a *Addon) Rollback(_ context.Context, _ ops.Target, _ ops.Env, _ *ops.Changeset) (*ops.RollbackInfo, error) {
	return &ops.RollbackInfo{}, nil
}

func (a *Addon) Harnesses(_ detect.Repo, _ ops.Target) ([]ops.Harness, error) {
	return nil, nil
}
