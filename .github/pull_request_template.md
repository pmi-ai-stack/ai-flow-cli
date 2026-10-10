## What and why

<!-- One logical change. Link the phase record: .ai/phase-N/plan.md in the workspace. -->

## Checklist

The spec's Working agreement (`spec-v7/ai-flow-implementation-spec.md` §Working agreement), as review items.

- [ ] Pre-PR gate is green: `gofmt -l .` → `go vet ./...` → `go tool golangci-lint run` → `go test -race -count=1 ./...` → `go mod tidy` (no diff)
- [ ] `ai-flow standards check .` passes against this repository
- [ ] Every new or changed command ships with its fixture or integration test in this PR
- [ ] Every new parser has a recorded real tool output under `testdata/`
- [ ] Every new or changed guard row has allow, deny and out-of-scope cases for both hosts
- [ ] Every new allowlist row has its class asserted (`TestAllowlistNeverMutatesPersistentState` still passes)
- [ ] Artifacts stay byte-stable: same inputs, same bytes
- [ ] If the envelope, a schema, the hook contract or the command catalog changed: `cli_version` bump noted for the plugin PR
- [ ] Spec updated in the same change when behaviour the spec describes changed
- [ ] Commits are one logical change each; the Go `go` line is not touched (the version bump is its own PR)
