package render

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

// Options is one Render call: materialize Target (as detected — Path
// relative to its repo) at Ref, whose content on disk is SourceDir
// (the caller's job to resolve: Repo's own working checkout for
// RefBranch, gitops.BaseWorktreePath for RefBase — this package knows
// nothing about git).
type Options struct {
	Addon     ops.Addon
	Target    ops.Target
	Ref       ops.Ref
	SourceDir string
	Home      string
	Key       string
}

// Result is what Render produced: the tree hash it memoized on, and
// RedactedDir — the ticket-relative path (under "render/") the agent
// and every later OPS stage reads.
type Result struct {
	Hash        string
	RedactedDir string
}

// Render materializes opts.Target at opts.Ref through opts.Addon,
// memoized by content-addressed tree hash (TreeHash of opts.SourceDir):
// a cache hit — for this ticket or any other — never calls
// opts.Addon.Render a second time for the same content. The real
// rendering lives at CachePath(opts.Home, hash), mode 0700; this call
// always (re)builds the redacted copy at
// <KEY>/render/<repo>/<addon>/<path>/<hash>/, redacted via
// redact.RedactMarked over the rendering's own sensitivity markings.
func Render(ctx context.Context, opts Options) (*Result, error) {
	hash, err := TreeHash(opts.SourceDir)
	if err != nil {
		return nil, err
	}

	realDir := CachePath(opts.Home, hash)
	marked, err := renderToCache(ctx, opts, hash, realDir)
	if err != nil {
		return nil, err
	}

	redactedDir, err := writeRedactedCopy(opts, hash, realDir, marked)
	if err != nil {
		return nil, err
	}

	return &Result{Hash: hash, RedactedDir: redactedDir}, nil
}

// renderToCache returns realDir's own markings, calling opts.Addon.Render
// into a temporary directory and renaming it into place only on a
// cache miss. The call-scoped Target passed to Addon.Render carries
// Path rewritten to opts.SourceDir — see the phase-19 plan's design
// decision 1 on why the Addon interface (frozen since phase 18) has no
// separate source parameter.
func renderToCache(ctx context.Context, opts Options, hash, realDir string) ([]redact.MarkedValue, error) {
	if dirExists(realDir) {
		return readMarkings(realDir)
	}

	root := tmpRoot(opts.Home)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("render: create %s: %w", root, err)
	}
	tmp, err := os.MkdirTemp(root, hash+"-*")
	if err != nil {
		return nil, fmt.Errorf("render: create temp dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()

	callTarget := opts.Target
	callTarget.Path = opts.SourceDir
	rendered, err := opts.Addon.Render(ctx, callTarget, opts.Ref, tmp)
	if err != nil {
		return nil, err
	}
	if err := writeMarkings(tmp, rendered.Marked); err != nil {
		return nil, err
	}

	// Another process (a concurrent `ops render`, possibly for a
	// different ticket sharing identical content) may have won the race
	// since the check above — re-check immediately before the rename
	// rather than locking (the plan's own design decision 5).
	if dirExists(realDir) {
		return readMarkings(realDir)
	}
	if err := os.MkdirAll(filepath.Dir(realDir), 0o700); err != nil {
		return nil, fmt.Errorf("render: create %s: %w", filepath.Dir(realDir), err)
	}
	if err := os.Rename(tmp, realDir); err != nil {
		return nil, fmt.Errorf("render: rename into place: %w", err)
	}
	cleanup = false
	if err := os.Chmod(realDir, 0o700); err != nil { //nolint:gosec // G302: 0700 is the spec's own mode for the rendering cache
		return nil, fmt.Errorf("render: chmod %s: %w", realDir, err)
	}
	return rendered.Marked, nil
}

// writeRedactedCopy copies every real file in realDir (except the
// private markings sidecar) into the ticket's own render/ directory,
// with every marked value replaced by its <redacted:...> token. absDir
// is created up front — via handoff.WriteFile for every non-empty
// subdirectory the walk below visits, and unconditionally here too, so
// a target whose rendering is empty still gets a real (if empty)
// directory rather than none at all.
func writeRedactedCopy(opts Options, hash, realDir string, marked []redact.MarkedValue) (string, error) {
	ticketDir := handoff.TicketDir(opts.Home, opts.Key)
	relDir := filepath.Join("render", targetDir(opts.Target), hash)
	absDir := filepath.Join(ticketDir, relDir)
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		return "", fmt.Errorf("render: create %s: %w", absDir, err)
	}

	err := filepath.WalkDir(realDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == markingsFile && filepath.Dir(path) == realDir {
			return nil
		}
		rel, err := filepath.Rel(realDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from WalkDir over realDir, a value this package itself computes
		if err != nil {
			return err
		}
		redacted := redact.RedactMarked(string(data), marked)
		destDir := filepath.Join(absDir, filepath.Dir(rel))
		if _, err := handoff.WriteFile(destDir, filepath.Base(rel), []byte(redacted)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("render: build redacted copy: %w", err)
	}
	return relDir, nil
}

// targetDir is the redacted copy's own directory segment for t:
// <repo>/<addon>/<path>, nested rather than flattened, so two targets
// that would otherwise collide (two addons at the same path; two
// repositories sharing a target path — phase 5's still-open monorepo
// question) never do (the plan's own design decision 1).
func targetDir(t ops.Target) string {
	return filepath.Join(t.Repo, t.Addon, filepath.FromSlash(t.Path))
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Load returns the *ops.Rendered (real cache directory plus its
// sensitivity markings) for a hash Render has already produced —
// phase 20's own way of handing Addon.Validate/Drift/DryRun the real
// rendering they must run against (Determinism §6: the real one is
// only reachable through Exec, and a validator/planner subprocess is
// part of that same trust boundary). The caller is expected to have
// gotten hash from a prior Render call in this same invocation; Load
// itself does not render anything and does not error when the cache
// entry is absent (CachePath simply reads as empty in that case, the
// same tolerant-of-absence shape ReadArtifact/readMarkings already
// use) — it is the caller's job to have rendered first.
func Load(home, hash string) (*ops.Rendered, error) {
	dir := CachePath(home, hash)
	marked, err := readMarkings(dir)
	if err != nil {
		return nil, err
	}
	return &ops.Rendered{Dir: dir, Marked: marked}, nil
}
