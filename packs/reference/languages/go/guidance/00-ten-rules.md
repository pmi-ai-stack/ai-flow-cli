# Go coding rules — `ai-flow` CLI

**Status:** rules / living document
**Date:** 2026-09-21
**Applies to:** every Go file in the `ai-flow-cli` module (`cmd/ai-flow`, `internal/...`, `schema/`, `testdata/`)
**Derived from:** this repo's own envelope/CLI/execution contract (`internal/envelope`, `internal/cli`, `internal/ops/exec` and their tests) and Effective Go, Go Code Review Comments, Google Go Style and Uber Go Style, in that order, where the code and its tests say nothing. See `sources.md`.

This is the reviewer's checklist for every Go file in `ai-flow-cli`. Every rule is either enforced by a tool (marked **[tool]**) or checked in review (marked **[review]**). A rule that cannot be stated as a check or a one-line review item does not belong here. It moved here, split by section, from `ai-flow-cli/docs/go-coding-rules.md` in phase 1; that file no longer exists.

## 0. The ten rules that matter most

1. **`gofmt`/`goimports` clean, `go vet` clean, `golangci-lint` clean.** Not discussed in review. **[tool]**
2. **stdout is the JSON envelope, nothing else.** Progress, logs, tool chatter go to stderr. A skill parses stdout blindly. **[review]**
3. **`os.Exit` only in `main`, once.** Every command is `RunE` and returns an error; `main` maps the error to an exit code (0–6) and the envelope. **[review]**
4. **Every error is checked, wrapped with `%w` and new context, and handled once** — returned *or* logged, never both. **[tool + review]**
5. **`ctx` first, timeout on every external call** (tracker, host, git, tool subprocess). Ctrl-C cancels everything through one `signal.NotifyContext`. **[review]**
6. **Every subprocess goes through `internal/ops/exec`** and its typed allowlist. No `exec.Command` anywhere else; no `sh -c`, ever. **[tool: `TestAllowlistNeverMutatesPersistentState` + `TestNoOsExecOutsideExecPackage`, both in `internal/ops/exec`]**
7. **Same inputs, same bytes.** JSON artifacts: sorted keys, two-space indent, trailing newline, no timestamps. Wall-clock data goes only to `events.jsonl`. **[tool: `handofftest.RequireByteStable`]**
8. **Every write to the handoff dir is atomic** (temp file in the same directory, then rename) and schema-validated before rename. **[review]**
9. **Secrets come from env only, and are redacted** from stderr, errors, `events.jsonl` and copied artifacts. Never in flags, never in config, never in the handoff dir. **[review + tests in `internal/redact`]**
10. **Tests run offline**: fixtures under `testdata/`, fake providers behind interfaces, no network, no tool binaries, `-race` always. Integration tests live behind build tags. **[tool]**

