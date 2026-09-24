// Package rollback derives the revert path a rollback stage records:
// git revert instructions per repository, each target's own
// tool-level rollback (via Addon.Rollback, already real since phases
// 21/22), the flat non_revertible union stage complete's plan_check
// reads, and the deterministic watch list. Every derived instruction
// is text only — never executed, and carrying no
// internal/ops/exec allowlist row, exactly like Addon.Rollback's own
// contract.
package rollback
