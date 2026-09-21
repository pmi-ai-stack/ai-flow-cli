## 8. Running tools: `internal/ops/exec`

The CLI is a subprocess orchestrator and **must never mutate a persistent target**. This package is the trust boundary.

```go
type Exec interface {
    Run(ctx context.Context, tool string, args []string, opts RunOpts) (Output, error)
}
```

- **The allowlist is a table of typed rows**, `[]Row{Tool, Verb, RequiredFlags, ForbiddenFlags, Mutating, Class}`, in `internal/ops/exec`. `Run` refuses anything matching no row with `*envelope.Error{Code: "exec_not_allowed"}` (exit 2) *before* looking up the binary, naming the equivalent `ai-flow` command in the message. This is a positive allowlist, not a verb blacklist: a mutating verb (a server-side dry run, an ephemeral test cluster's lifecycle) can appear in a row when it is provably safe by construction. **[tool: `TestAllowlistNeverMutatesPersistentState` — every row with `Mutating: true` must carry its own dry-run flag in `RequiredFlags` (class `none`/`read_only_live`), or be `class: ephemeral` (acts only on the ticket's own `kind` cluster or a `sandbox`-tier workspace), or be `class: repo_write` (the local git verbs are reachable only from `git prepare`/`checkout-base`/`checkout-branch`; the publishing ones only from `git commit`/`git push`, gated on a recorded `ship` approval) — CI fails otherwise]**
- **`exec.Command`/`exec.CommandContext` appear in exactly one package in the module: `internal/ops/exec` itself.** `TestNoOsExecOutsideExecPackage` (in that package, runs under plain `go test`, no CI-only grep needed) fails on any other hit. Standards check runners, addons, harness runners, `gitops` and `standards sync` (git) all go through `Exec`.
- **Never `sh -c`, never a shell.** Arguments are a `[]string`; globbing is `filepath.Glob` in Go.
- `exec.LookPath` up front → `tool_missing` (exit 4) with a hint naming the binary and the doc page, instead of a wrapped `executable file not found`.
- `cmd.Env` is built explicitly: inherit `os.Environ()`, then set/override `KUBECONFIG`, `TF_WORKSPACE`, `HELM_CACHE_HOME`, `TF_IN_AUTOMATION=1`, `NO_COLOR=1` etc. per call; never mutate the process env with `os.Setenv` outside tests.
- Capture stdout/stderr into buffers; stream stderr to the logger at `Debug` through `io.MultiWriter`; drain pipes *before* `Wait`. Large outputs (`terraform show -json`) are written straight to their destination file, not held in memory twice.
- Output is redacted before it is stored in `Output`, in the `ExitError`, or logged.
- `RunOpts` carries `Dir`, `Env`, `Stdin`, `Timeout`; the timeout default comes from config (`ops.tool_timeout_minutes`).
- Consumers depend on the **interface**, not the struct, so tests inject a fake that records calls and replays fixtures from `testdata/`.
