package render_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

func TestTerraformPluginCacheDir(t *testing.T) {
	got := render.TerraformPluginCacheDir("/home/x/.ai-flow")
	want := filepath.Join("/home/x/.ai-flow", "cache", "terraform")
	if got != want {
		t.Errorf("TerraformPluginCacheDir = %q, want %q", got, want)
	}
}

func mkCacheEntry(t *testing.T, home, hash string) {
	t.Helper()
	dir := render.CachePath(home, hash)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCacheEntriesListsEveryEntryAndSkipsTmp(t *testing.T) {
	home := t.TempDir()
	mkCacheEntry(t, home, "hash-a")
	mkCacheEntry(t, home, "hash-b")
	// A .tmp directory (Render's own staging area) must never be
	// reported as a real entry.
	if err := os.MkdirAll(filepath.Join(home, "cache", "render", ".tmp"), 0o700); err != nil {
		t.Fatal(err)
	}

	entries, err := render.CacheEntries(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("CacheEntries = %v, want 2 entries", entries)
	}
	var hashes []string
	for _, e := range entries {
		hashes = append(hashes, e.Hash)
	}
	for _, want := range []string{"hash-a", "hash-b"} {
		if !slices.Contains(hashes, want) {
			t.Errorf("CacheEntries missing %q, got %v", want, hashes)
		}
	}
}

func TestCacheEntriesEmptyWhenCacheAbsent(t *testing.T) {
	home := t.TempDir()
	entries, err := render.CacheEntries(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("CacheEntries = %v, want none (no cache directory at all)", entries)
	}
}

// TestPruneRemovesOnlyEntriesKeepRejects proves the retention hook's
// mechanism works even though it ships with no policy of its own —
// phase 30 supplies keep; this phase only proves the removal is
// correct once told what to remove.
func TestPruneRemovesOnlyEntriesKeepRejects(t *testing.T) {
	home := t.TempDir()
	mkCacheEntry(t, home, "keep-me")
	mkCacheEntry(t, home, "remove-me")

	removed, err := render.Prune(home, func(e render.CacheEntry) bool {
		return e.Hash == "keep-me"
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "remove-me" {
		t.Errorf("Prune removed = %v, want [remove-me]", removed)
	}

	entries, err := render.CacheEntries(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Hash != "keep-me" {
		t.Errorf("CacheEntries after Prune = %v, want only keep-me", entries)
	}
}

func TestPruneKeepEverythingRemovesNothing(t *testing.T) {
	home := t.TempDir()
	mkCacheEntry(t, home, "a")
	mkCacheEntry(t, home, "b")

	removed, err := render.Prune(home, func(render.CacheEntry) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Errorf("Prune removed = %v, want none", removed)
	}
}
