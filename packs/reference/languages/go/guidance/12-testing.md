## 12. Testing

Tests are why the mechanics moved out of prompts and into a binary. Every command ships with its test in the same PR.

- **Unit tests run offline, with no tool binaries, in milliseconds**: `go test ./...` must pass on a machine with only Go installed. **[tool: CI job without `terraform`/`kubectl`/`helm` on PATH]**
- **Fixtures under `testdata/`**: recorded HTTP responses (Jira, GitHub), recorded tool output (`terraform show -json`, `kubectl diff`, `helm diff`, scanner JSON, linter JSON, mutation reports), sample repos per addon, sample packs (valid, conflicting, over-budget). Every parser has at least one *real* recorded output. Fixture files are named `<tool>-<version>-<scenario>.<ext>` so version drift is visible.
- **Golden files** for every rendered artifact (`output.md`, `standards/<stage>.md`, `pr-body` OPS block, normalized plan JSON): `testdata/*.golden`, `-update` flag, diff reviewed in git. Compare with `cmp.Diff` for structs and a line diff for text.
- **Byte-stability tests** (`handofftest.RequireByteStable`, with `handofftest.FixedClock`) in each writer's own tests — `handoff`, `mutation`, `ops/...`, `standards`, `render`: run the writer twice on the same input (and once with shuffled input order) and assert identical bytes.
- **Table tests** with named fields and `t.Run`; `t.Parallel()` where the test does not use `t.Setenv`/`t.Chdir`. Failure messages carry input, got and want. **[tool: `thelper`, `paralleltest` optional]**
- **Fakes over mocks.** A hand-written `fakeExec` that records calls and replays fixtures; a `fakeTracker` with an in-memory ticket. No mocking framework.
- `t.TempDir()` for every file test; `t.Setenv` for every env var; never depend on `$HOME`, cwd, a real kubeconfig or network.
- **Build tags for the slow tiers**, exactly as the spec names them: `//go:build integration` (real git in temp repos), `//go:build mutation`, `//go:build ops` (real tools when on PATH, else `t.Skip` with a notice). They run in their own CI jobs.
- **End-to-end CLI tests with `testscript`** (`github.com/rogpeppe/go-internal/testscript`): one `.txtar` per command family that execs the built binary, runs a scenario against fixtures and `cmp`s stdout with the expected envelope. This is the test a skill author reads to learn the contract.
- **Mandatory named tests** from the spec, kept under those exact names so CI can `-run` them: `TestAllowlistNeverMutatesPersistentState` and `TestNoOsExecOutsideExecPackage` (`internal/ops/exec`), risk table test with every weight cell and the exact `risk_gate` boundary (`internal/ops/risk`), resolver tests for layering/override/waiver-expiry/budget-drop order (`internal/standards`), exit-code table completeness (`internal/envelope`), state-machine transition table (`internal/handoff`).
- **Fuzz** the hand-rolled parsers (`ticket key sanitizer`, branch-name derivation, plan-JSON normalizer, conventional-commit check).
- **Dogfood mutation testing**: `internal/mutation`, `internal/handoff`, `internal/ops/risk`, `internal/standards` are held to the default threshold (80) with `gremlins` in CI. Write tests that kill mutants, not tests that raise coverage.
- Coverage is a tool, not a target: aim it at the fragile logic (scoring, resolver, state machine, normalizers), not at `cmd/`.

