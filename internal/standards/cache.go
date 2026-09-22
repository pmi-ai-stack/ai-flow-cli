package standards

import "path/filepath"

// CacheDir returns the fixed location a source's synced pack content
// lives at: ~/.ai-flow/standards/<id>@<ref>/ — under the "standards"
// reserved name internal/handoff.ReservedNames already carries, the
// same "reserved-name, non-ticket path helper owned by the package that
// uses it" placement internal/gitops.BaseWorktreePath established for
// "cache".
func CacheDir(home, id, ref string) string {
	return filepath.Join(home, "standards", id+"@"+ref)
}

// SyncMetaPath returns the sidecar file that records when a source was
// last synced and how its ref was classified — deliberately outside
// CacheDir itself, so the synced pack's own tree never carries sync
// bookkeeping as spurious content (see the phase-12 plan's design
// decision 5: syncing the same commit by tag, by hash and from a local
// path must produce byte-identical trees).
func SyncMetaPath(home, id, ref string) string {
	return filepath.Join(home, "standards", ".sync", id+"@"+ref+".json")
}
