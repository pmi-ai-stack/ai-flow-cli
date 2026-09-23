// Package standards owns everything about standards packs: the pack
// format's loader, structural validator and sync mechanism (phase 12 —
// manifest.go, validate.go, sync.go, export.go), and the resolver built
// on top of them (phase 13 — pack.go, layers.go, applies.go,
// resolve.go, budget.go, waivers.go, hash.go, guidance.go,
// artifact.go): layering org → team → repo, precedence and declared
// overrides, waivers, the per-stage token budget, the resolution hash,
// and the standards.json / guidance.md writers.
//
// It still runs nothing. Executing a check — the CheckRunner interface
// and its eight runners — is phase 14, and lands alongside this code,
// never inside Resolve.
package standards
