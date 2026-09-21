## 2. Layout and packages

Layout is fixed by the spec; do not invent directories:

```
cmd/ai-flow/main.go            signal ctx, build root cmd, ExecuteContext, print envelope, os.Exit — nothing else
internal/cli/                  cobra commands, one file per noun (ticket.go, repo.go, git.go, stage.go, ops.go, standards.go, …)
internal/envelope/             the JSON envelope type, error → exit-code mapping, needs_input
internal/config/               load, validate, embedded template, v1→v2→v3→v4 upgrades
internal/handoff/              paths, atomic write, status state machine, events.jsonl
internal/schema/               embedded JSON Schemas + validate()
internal/tracker/, tracker/jira/
internal/host/, host/github/
internal/gitops/  routing/  detect/  mutation/{stryker,mutmut,gremlins,pitest}/
internal/ops/                  Addon interface, registry, Target, Env, Changeset, kind classes
internal/ops/exec/             the only place that runs a binary
internal/ops/{envs,risk,policy,rollback,harness}/
internal/ops/addons/{terraform,kubectl,helm,kustomize,githubactions,jenkins}/
internal/standards/, standards/runners/{format,lint,policy,dependency,secrets,layout,docs,commit,pr}/
internal/render/  redact/
schema/*.schema.json
testdata/
```

Rules:

- **`cmd/ai-flow/main.go` is ≈ 30 lines** and has no business logic. Everything else is under `internal/`; nothing is importable from outside the module until another repo actually needs it. **[review]**
- **No `pkg/`, `util/`, `common/`, `helpers/`, `misc/`, `models/`, `types/`.** Packages are named by what they provide (`handoff`, `risk`, `redact`), lowercase, singular, one word, no underscores. **[review]**
- **Group by concept, not by layer.** `internal/ops/risk` owns scoring *and* its types *and* its tests; there is no `internal/services`. **[review]**
- **No stutter:** `risk.Score`, not `risk.RiskScore`; `handoff.Write`, not `handoff.WriteHandoff`. Getters have no `Get`. **[review]**
- **Dependency direction:** `cli` → domain packages → `exec`/`tracker`/`host`/`gitops`. Domain packages (`risk`, `standards`, `routing`, `mutation`) never import `cli`, `cobra`, or a provider package (`jira`, `github`). OPS stage code never imports an addon package; it sees only `Changeset`, `Rendered`, `DriftReport`, `PolicyFindings`, `RollbackInfo`, `TestResult`. Circular imports mean the split is wrong, not that you need an interface. **[review; consider `depguard` in golangci when the tree stabilizes]**
- One `doc.go` (or a package comment on the main file) per package: `// Package risk computes the deterministic risk score of a changeset.`
- Don't split a package until it hurts. A 1,500-line package with clear file names is fine.

