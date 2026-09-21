## Sources

- This repo's own contract — `internal/envelope` (envelope, exit codes), `internal/cli` (command layer, layout), `internal/ops/exec` (the execution allowlist, determinism rules) — and their tests, which are authoritative. Where this document and the code disagree, fix whichever is wrong in the same PR.
- General Go rulebooks consulted when drafting this document (outside this workspace, in the sibling `core-platform` checkout): `core-platform/.local/learn/04-go-best-practices-for-devops.md` (§5–§16 there map onto §2–§13 here), `core-platform/.local/learn/02-go-for-devops.md` (checklist and anti-patterns), `core-platform/.local/learn/01-modern-programming-best-practices.md` (the "why": hexagonal layering, 12-factor, fail fast)
- Effective Go — https://go.dev/doc/effective_go
- Go Code Review Comments — https://go.dev/wiki/CodeReviewComments
- Google Go Style: Decisions / Best Practices — https://google.github.io/styleguide/go/
- Uber Go Style Guide — https://github.com/uber-go/guide/blob/master/style.md
- Organizing a Go module — https://go.dev/doc/modules/layout
- golangci-lint v2 configuration — https://golangci-lint.run/docs/configuration/file/
- Cobra: working with commands, context — https://cobra.dev/docs/how-to-guides/
- testscript — https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript
