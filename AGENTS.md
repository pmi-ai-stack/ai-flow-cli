# AGENTS.md — ai-flow-cli

This repo is the **CLI**: the sole deterministic actor in the ai-flow pipeline. Every side effect (tracker, git, git host, infra tools, test/mutation runners) goes through this binary; nothing else calls a provider directly.

- **The plugin** (skills, hooks, agent-facing manifests) is a sibling repository, `ai-flow`.
- **The contract this binary implements** — command catalog, envelope/exit-code shape, config schema, handoff layout, addon interface — is defined by its own code and tests, which are authoritative: `internal/envelope` (envelope shape, exit codes), `internal/cli` (commands, global flags), `internal/ops/exec` (the execution allowlist). Read the relevant package before adding or changing any command, schema, config field or exit code; there is no separate design document to consult.
- **Go rules:** `packs/reference/languages/go/guidance/*.md` — the reviewer's checklist for every Go file in this module, split by topic (`00-ten-rules.md` first). Follow it; do not duplicate its rules here.

## Pre-PR command

```
gofmt -l . && go vet ./... && go tool golangci-lint run && go test -race -count=1 ./... && go mod tidy
```

(`go mod tidy` must produce no diff.) Once `internal/standards` and its check runners exist, also run `ai-flow standards check .` against this repo's own reference pack.

## What this binary is

The agent (Claude Code / Cursor skills) never talks to Jira/GitHub/git/terraform/kubectl/helm directly, never hand-writes machine-readable state, and never decides which standards apply — it only calls `ai-flow <noun> <verb> --json` and reads the JSON envelope from stdout. The CLI owns every side effect; the agent owns judgement (planning, coding, writing tests, interpreting results).

Key invariants, enforced in the packages named:

- The CLI never calls a model and never asks interactive questions — ambiguity is always exit 4 with a `needs_input` payload (`internal/envelope`).
- Every subprocess goes through `internal/ops/exec`'s typed allowlist; nothing on the allowlist can mutate a persistent infrastructure target (`TestAllowlistNeverMutatesPersistentState`, in that package).
- Provider integrations are reachable only through interfaces (`Tracker`, `Host`, `MutationTool`, `Addon`, `Exec`) — a new provider is a new package plus a config value, never a change to calling code.
- Determinism throughout: JSON artifacts are written with sorted keys, atomic writes (temp file + rename), no timestamps in content, so identical inputs always produce identical bytes.

## Repository status

Built so far: `go.mod`, `cmd/ai-flow/main.go`, `internal/envelope`, `internal/ops/exec` (allowlist type + the production `Table` — still empty — and `Hash()`), the top-level `schema/` package (17 embedded JSON Schemas) with `internal/schema` as its loader/validator, `internal/redact` (environment-variable secret discovery, `<redacted:sha256-prefix>` substitution, the marking-driven hook phase 19 will use), `internal/config` (the full `config.json` type set, embedded template, `Load`, `Validate`), `internal/handoff` (the pipeline's spine: ticket-key/path helpers, the byte-stable atomic writer every artifact writer builds on, the sixteen-stage/seven-state machine — `Start`/`Complete`/`Approve`/`Skip`/`Reset` — over `status.json`, the singular `active.json` and each ticket's `events.jsonl`, `Catalog()`/`StageTable()`, and the `must_findings_open`/`output_not_passed`/`ship_approved` predicates), `internal/handoff/handofftest` (the `FixedClock`/`RequireByteStable` harness every later artifact-writer's tests use — note: a package's own *internal* tests cannot import this back, only an external `_test` package or a different package can; see `internal/handoff`'s own test files for the workaround), `internal/cli` (root command + `version`, `schema <artifact>`, `config init`/`config validate`, `status`, `stage should-run/start/complete/approve/skip/reset`), the Go guidance pack, release tooling and CI. Not yet built: the tracker/host/gitops/detect/verify/mutation packages, standards, OPS. Deferred within what exists: real skip-rule evaluation for the nine stages whose rule needs data from phases 6/14/18/20/23/24 (`ShouldRun` always answers `run: true`, carrying the stage's coded rule id); `rollback`'s `plan_check`; the tree-hash staleness refusal for `impact`/`rollback` re-completion; heading/prose validation of agent-written Markdown artifacts. `internal/config`'s `Config` never carries a secret — tracker/host tokens will be read from the environment directly by the packages that need them, not merged into it. Check this repo's own package tree (`internal/*`) before assuming a command or package exists — if the package isn't there, the command isn't either.
