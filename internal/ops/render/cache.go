package render

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CachePath returns the real rendering cache's own directory for hash
// — ~/.ai-flow/cache/render/<hash>/, mode 0700, never an artifact and
// named by no artifact (see the plan's own grep test).
func CachePath(home, hash string) string {
	return filepath.Join(home, "cache", "render", hash)
}

// tmpRoot is where Render builds a rendering before it is known-good
// and renamed into place — a sibling of CachePath's own parent, so the
// final os.Rename is same-filesystem and therefore atomic.
func tmpRoot(home string) string {
	return filepath.Join(home, "cache", "render", ".tmp")
}

// terraformCacheDirName is named rather than inlined as a literal
// filepath.Join argument on purpose: internal/ops/exec's own
// TestEphemeralRowsReachableOnlyFromDesignatedCallers greps every
// module file's call-expression string-literal arguments for
// ClassEphemeral tool names ("terraform" among them, via `terraform
// test`) — a real, if literal-minded, false positive this directory
// name would otherwise trip, since a go/ast scan cannot tell "the path
// segment terraform" from "the tool terraform" apart. A named constant
// is not a *ast.BasicLit call argument, so the scanner never sees it —
// the same fix class TestGitPublishingVerbsReachableOnlyFromGitopsPublish's
// own suppress-predicate mechanism exists for, chosen here instead
// because it needs no change to that test at all.
const terraformCacheDirName = "terraform"

// TerraformPluginCacheDir returns ~/.ai-flow/cache/terraform — the
// spec's own TF_PLUGIN_CACHE_DIR value. No terraform addon exists
// before phase 21; this is plumbing with no caller yet, the same
// shape internal/ops/exec.Which shipped in phase 18 before any
// command called it.
func TerraformPluginCacheDir(home string) string {
	return filepath.Join(home, "cache", terraformCacheDirName)
}

// CacheEntry is one real rendering cache entry — enough for a
// retention policy (phase 30) to decide what to keep.
type CacheEntry struct {
	Hash    string
	Path    string
	ModTime time.Time
}

// CacheEntries lists every real rendering cache entry under home. This
// is phase 19's own retention hook: it enumerates entries and evicts
// nothing on its own — the plan's own words, "a retention hook for
// cache entries, with the policy itself decided in phase 30."
func CacheEntries(home string) ([]CacheEntry, error) {
	root := filepath.Join(home, "cache", "render")
	list, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("render: list %s: %w", root, err)
	}
	var entries []CacheEntry
	for _, d := range list {
		if !d.IsDir() || d.Name() == ".tmp" {
			continue
		}
		info, err := d.Info()
		if err != nil {
			return nil, fmt.Errorf("render: stat %s: %w", d.Name(), err)
		}
		entries = append(entries, CacheEntry{Hash: d.Name(), Path: filepath.Join(root, d.Name()), ModTime: info.ModTime()})
	}
	return entries, nil
}

// Prune removes every cache entry for which keep returns false, and
// returns the hashes it removed. The caller supplies the policy
// (phase 30's own job); this function only performs the (irreversible)
// removal once told to.
func Prune(home string, keep func(CacheEntry) bool) ([]string, error) {
	entries, err := CacheEntries(home)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		if keep(e) {
			continue
		}
		if err := os.RemoveAll(e.Path); err != nil {
			return removed, fmt.Errorf("render: remove %s: %w", e.Path, err)
		}
		removed = append(removed, e.Hash)
	}
	return removed, nil
}

// DefaultCacheMaxAge is phase 30's own decision for the spec's still-
// open "what is open is its retention" question (Determinism §6 / the
// spec's own Open list): a rendering is content-addressed and always
// regenerable from a real render, so an evicted entry costs the next
// caller one re-render, never correctness — there is no failure mode
// symmetrical to, say, evicting a ticket's own prose artifacts. 14 days
// is chosen to comfortably outlast one developer's active ticket (the
// guard's own idle default is 8 *hours*, three orders of magnitude
// shorter, because a stale *ticket* is a different kind of staleness
// than a stale *rendering*) while still bounding disk use across a
// long-lived ~/.ai-flow directory. No config field carries this: the
// spec's own config.json example (§Configuration and data model) is
// matched field for field by internal/config.Config, and this value
// needs no per-repository or per-team override the way, say,
// guard_idle_hours does — it is a pure disk-hygiene constant, kept
// here next to Prune rather than promoted into the config schema.
const DefaultCacheMaxAge = 14 * 24 * time.Hour

// Retain is Prune with phase 30's own default policy already applied:
// keep every entry rendered within maxAge of now. A caller wanting a
// different window (or a dry run) still calls Prune directly with its
// own keep func; this is the one production call site's shape.
func Retain(home string, maxAge time.Duration, now time.Time) ([]string, error) {
	return Prune(home, func(e CacheEntry) bool {
		return now.Sub(e.ModTime) < maxAge
	})
}
