// Package check runs one resolved standards check against one
// repository and produces checks-results.json's own finding shape.
// This is the one package in the standards tree that starts a
// subprocess (format/lint/secrets) or reads repository content
// directly off disk (dependency/layout/docs/commit/pr) — the resolver
// itself (internal/standards) never does either.
//
// A CheckRunner is chosen by the check's Kind, not by its Tool: a
// "lint" check can name golangci-lint, eslint, ruff or checkstyle
// depending on which language it applies to, so the dispatch a caller
// wants is "run whatever this kind means," with the kind's own runner
// then dispatching again on Tool where more than one binary can serve
// it. Mirrors internal/verify/unit's "one package, many parsers,
// dispatch by name" shape rather than internal/mutation's
// one-subpackage-per-tool shape, since no tool here owns a kind the
// way one mutation tool owns one language.
package check

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// Repo is the subset of repo.json a runner needs: enough to run a
// subprocess in the right directory and to name it in a Finding.
type Repo struct {
	Name      string
	LocalPath string
}

// Finding mirrors checks-results.schema.json's own finding shape,
// field for field. CheckID/Layer/Kind/Severity/Tool are filled in by
// the caller (internal/cli/checks.go) from the EffectiveCheck that
// produced them, not by the runner itself — a runner only ever knows
// about the one check it was asked to run, never its layer or its
// place in the resolution.
type Finding struct {
	CheckID        string `json:"check_id"`
	Layer          string `json:"layer"`
	Kind           string `json:"kind"`
	Stage          string `json:"stage"`
	Severity       string `json:"severity"`
	Tool           string `json:"tool"`
	Repo           string `json:"repo,omitempty"`
	Target         string `json:"target,omitempty"`
	File           string `json:"file,omitempty"`
	Line           int    `json:"line,omitempty"`
	Message        string `json:"message"`
	FixHint        string `json:"fix_hint,omitempty"`
	AutofixApplied bool   `json:"autofix_applied,omitempty"`
	Status         string `json:"status"`
}

// RunInput is everything a runner needs to check one repository
// against one resolved check.
type RunInput struct {
	// Check is the resolved check to run — its Kind selects the runner,
	// its Tool (when it has one) selects the specific binary, its
	// Config (when it has one) is a pack-relative path read through
	// Check.Pack.FS.
	Check standards.EffectiveCheck
	// Repo is the repository to check. Empty (Repo{}) for a pack-wide
	// check that is not per-repository (secrets, docs, commit, pr) —
	// each of those runners reads its own input from TicketDir instead.
	Repo Repo
	// Stage is "checks" or "ship" — the finding's own Stage field.
	Stage string
	// Fix requests the check's own autofix, honoured only by
	// Kind == "format" (see format.go's doc comment).
	Fix bool
	// TicketDir is the ticket's handoff directory, empty for the
	// directory (ticket-free) form of `standards check`. docs, commit
	// and pr read ticket artifacts from here and produce no findings at
	// all when it is empty (design decision 4 in the phase-14 plan) —
	// there is nothing under a bare directory for them to check.
	TicketDir string
}

// CheckRunner runs one check against one input.
type CheckRunner interface {
	Run(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error)
}

// CheckRunnerFunc adapts a function to CheckRunner, mirroring
// http.HandlerFunc — no runner in this package carries state across
// calls, so a bare function value is the whole implementation.
type CheckRunnerFunc func(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error)

// Run implements CheckRunner.
func (f CheckRunnerFunc) Run(ctx context.Context, in RunInput, x exec.Execer, timeout time.Duration) ([]Finding, error) {
	return f(ctx, in, x, timeout)
}

// runners is the kind → runner dispatch table. A pack may declare a
// check whose kind this binary does not implement (none can today —
// the eight kinds here are exactly the pack schema's own enum, and
// policy/scan are phase 25's) but the table is still keyed by string,
// not by an enum, so a later phase's addition is one more map entry.
var runners = map[string]CheckRunner{
	"format":     CheckRunnerFunc(runFormat),
	"lint":       CheckRunnerFunc(runLint),
	"dependency": CheckRunnerFunc(runDependency),
	"secrets":    CheckRunnerFunc(runSecrets),
	"layout":     CheckRunnerFunc(runLayout),
	"docs":       CheckRunnerFunc(runDocs),
	"commit":     CheckRunnerFunc(runCommit),
	"pr":         CheckRunnerFunc(runPR),
}

// ForKind returns kind's runner, and whether one is registered.
func ForKind(kind string) (CheckRunner, bool) {
	r, ok := runners[kind]
	return r, ok
}

// materializeConfig reads check's Config (a path relative to its
// Pack.FS — the embedded reference pack, a synced pack or a
// repository's own in-tree pack all read the same way) into a real
// temp file, since every external tool needs a real filesystem path
// for its --config/-c flag and the embedded pack has no path of its
// own on disk. The caller removes the returned path when done.
//
// The path is deterministic — <tmp>/ai-flow-check-config-<check
// id><ext> — rather than a fresh random name each call, the same
// reason internal/verify/unit's pytest report path is fixed rather
// than temp-random: a caller's own test scripts the exact argv it
// expects an exec.Execer to receive (exectest.Scripted matches argv
// verbatim), which a random suffix would make impossible to assert
// against. The original file's extension is preserved because some
// tools (gitleaks confirmed by hand against the real binary; eslint's
// flat config format is documented to need one too) infer their
// config's format from it and fail outright on an extensionless path.
// Two concurrent runs of the *same* check id on the same machine would
// race on this path — an accepted limitation for a single-developer
// local CLI, not a shared build farm concern this phase's mandate
// covers.
//
// Returns ("", nil, nil) when the check carries no Config at all — a
// built-in check (dependency/layout use their own Config directly,
// never through this helper; docs/commit/pr have none).
func materializeConfig(c standards.EffectiveCheck) (path string, cleanup func(), err error) {
	if c.Config == "" {
		return "", func() {}, nil
	}
	data, err := fs.ReadFile(c.Pack.FS, c.Config)
	if err != nil {
		return "", nil, fmt.Errorf("check: read config %s for %s: %w", c.Config, c.ID, err)
	}
	path = filepath.Join(os.TempDir(), "ai-flow-check-config-"+c.ID+filepath.Ext(c.Config))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", nil, fmt.Errorf("check: write config for %s: %w", c.ID, err)
	}
	return path, func() { _ = os.Remove(path) }, nil
}

// readConfigJSON reads and unmarshals a built-in check's own Config
// file (dependencies.json, layout.json) directly through the pack's
// FS — no temp file needed, since a built-in reads the config itself
// rather than handing a path to a subprocess.
func readConfigJSON(c standards.EffectiveCheck, v any) error {
	data, err := fs.ReadFile(c.Pack.FS, c.Config)
	if err != nil {
		return fmt.Errorf("check: read config %s for %s: %w", c.Config, c.ID, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("check: config %s for %s does not parse: %w", c.Config, c.ID, err)
	}
	return nil
}
