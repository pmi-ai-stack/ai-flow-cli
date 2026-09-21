## Sources

- This repo's own contract — `internal/envelope` (envelope, exit codes), `internal/cli` (command layer, layout), `internal/ops/exec` (the execution allowlist, determinism rules) — and their tests, which are authoritative. Where this document and the code disagree, fix whichever is wrong in the same PR.
- General Go rulebooks consulted when drafting this document (outside this workspace, in the sibling `core-platform` checkout): `core-platform/.local/learn/04-go-best-practices-for-devops.md` (§5–§16 there map onto §2–§13 here; its §5.1/§5.2 map onto §18 here — naming, doc comments, early-return control flow — its §5.3/§5.4 map onto §20 — shadowing and nil-vs-empty-slice, string gotchas — its §5.7 maps onto §19 — generics — and its §11 config-precedence note maps onto §21), `core-platform/.local/learn/02-go-for-devops.md` (checklist and anti-patterns), `core-platform/.local/learn/01-modern-programming-best-practices.md` (the "why": hexagonal layering, 12-factor, fail fast), `core-platform/kube-core-addons/docs/go-rules-for-agent.md` (the sibling project's own condensed enforceable checklist — cross-checked against §18's naming rules only; it does not cover generics, string gotchas, shadowing or config precedence)
- Effective Go — https://go.dev/doc/effective_go
- Go Code Review Comments — https://go.dev/wiki/CodeReviewComments
- Google Go Style: Decisions / Best Practices — https://google.github.io/styleguide/go/
- Uber Go Style Guide — https://github.com/uber-go/guide/blob/master/style.md
- Organizing a Go module — https://go.dev/doc/modules/layout
- golangci-lint v2 configuration — https://golangci-lint.run/docs/configuration/file/
- Cobra: working with commands, context — https://cobra.dev/docs/how-to-guides/
- testscript — https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript
