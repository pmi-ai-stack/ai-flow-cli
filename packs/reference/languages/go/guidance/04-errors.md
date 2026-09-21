## 4. Errors

```go
var ErrStageNotAllowed = errors.New("stage transition not allowed")

type ExitError struct { // from internal/ops/exec
    Tool   string
    Args   []string
    Code   int
    Stderr string // already redacted
}
func (e *ExitError) Error() string { return fmt.Sprintf("%s: exit status %d: %s", e.Tool, e.Code, strings.TrimSpace(e.Stderr)) }
```

1. Check every error immediately. `errcheck` fails CI. **[tool]**
2. Wrap with `fmt.Errorf("render %s@%s: %w", target.ID, ref, err)`. Add what *this* layer knows (ticket key, repo, target, stage, file), not what the callee already said. The chain should read like a stack: `ops dry-run: target infra/prod: terraform plan: exit status 1: …`.
3. Error strings: lowercase, no trailing punctuation, no `failed to`/`error:` prefix. **[tool: `staticcheck ST1005`]**
4. Sentinel errors for conditions callers branch on (`ErrNotFound`, `ErrDirtyWorktree`); compare with `errors.Is`, never `==` or string matching. **[tool: `errorlint`]**
5. Typed errors when the caller needs data (`*exec.ExitError`, `*schema.ValidationError` with pointer paths, `*envelope.Error`). Extract with `errors.As`.
6. **Handle once.** The top of the program (`main`) is the only place that prints an error; `--verbose` logs are diagnostics, not error handling.
7. `%v` instead of `%w` only at a deliberate boundary (e.g. when converting a provider SDK error into `tracker_auth` so callers don't depend on the SDK's type).
8. `errors.Join` for "collect every validation failure, then fail" — `config validate`, `standards validate`, schema validation all return the full list, not the first hit.
9. `panic` only for programmer errors at package init (bad embedded schema, bad regexp constant) via a `must*` helper; never on input, files, tool output or network. No `recover` to hide bugs. **[review]**
10. Never `nil, nil`. Never `_ = err` without a `//` comment saying why.
11. A subprocess exit code ≠ 0 is an `*ExitError`, not a result. Distinguish *tool failed* (exit 5 `tool_failed`), *tool missing* (exit 4 `tool_missing`) and *we cancelled* (`ctx.Err()`).

