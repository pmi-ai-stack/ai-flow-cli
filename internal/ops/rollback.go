package ops

// RollbackInfo is Addon.Rollback's result: the revert path as text
// only, never executed and carrying no allowlist row (phase 23 owns
// the derivation and non-revertible classification for real).
type RollbackInfo struct {
	Steps         []string
	NonRevertible []string
}
