package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// TreeHash is a content-addressed fingerprint of every file under root,
// recursively, except beneath a directory skipDir names (nil skips
// nothing; root itself is never skipped — a .github/workflows target's
// own root is a dot-directory): sha256 over the sorted list of
// "<path-relative-to-root>\x00<sha256 of content>\n" lines. Identical
// content hashes identically no matter which ticket, ref or checkout
// it was read from — the property Render's own memoization and phase
// 20's "re-render every target whose tree hash moved" both depend on.
//
// This reimplements internal/standards.Fingerprint's own walk-hash-sort
// algorithm rather than calling it (os.DirFS(root) could bridge a real
// directory to the fs.FS that function wants, so that is not the
// binding constraint): Fingerprint returns a "sha256:"-prefixed digest
// (its own callers all write it into a JSON artifact, where that
// spelling matters), which is not usable as-is for CachePath's own
// directory-name path segment — a colon there is legal on Linux but
// not portable to Windows, a platform this project ships binaries for.
// Importing internal/standards from this package for the unprefixed
// core alone would also be a real, if smaller, coupling — see the
// phase-19 plan's design decision 4.
//
// skipDir must be the predicate the target's own addon Render copies
// with (SkipFor) — a mismatch either hashes content the rendering never
// sees (a needless re-render whenever it moves) or misses content it
// does (a stale cache hit). Symlinks are followed, as every addon's
// copyTree follows them; FIFOs, sockets and devices are skipped, since
// reading a FIFO would block forever and none is render input.
func TreeHash(root string, skipDir func(name string) bool) (string, error) {
	var lines []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir != nil && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from WalkDir over root, a caller-supplied source checkout/worktree, not attacker input
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, filepath.ToSlash(rel)+"\x00"+hex.EncodeToString(sum[:])+"\n")
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("render: tree hash %s: %w", root, err)
	}
	slices.Sort(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SkipFor returns a's directory-skip predicate for TreeHash, or nil
// when a does not implement ops.TreeSkipper (hash everything).
func SkipFor(a ops.Addon) func(name string) bool {
	if s, ok := a.(ops.TreeSkipper); ok {
		return s.SkipTreeDir
	}
	return nil
}
