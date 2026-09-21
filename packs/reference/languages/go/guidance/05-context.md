## 5. Context, timeouts, cancellation

- `ctx context.Context` is the first parameter of anything that does I/O, waits, or spawns a process. Never stored in a struct (Cobra's `cmd.Context()` is the one accepted exception). Never `nil`.
- Every external call has a deadline: `context.WithTimeout(ctx, cfg.Ops.ToolTimeout)`; the value comes from config (`ops.tool_timeout_minutes`, HTTP client timeouts for tracker/host), never a literal in the call site. **[review]**
- `http.Client` always has `Timeout` set; there is exactly one constructor for the tracker and one for the host client.
- Subprocesses use `exec.CommandContext` **and** `cmd.Cancel` (SIGTERM first) + `cmd.WaitDelay` so `terraform plan` gets a chance to release its lock on Ctrl-C.
- `context.WithoutCancel(ctx)` for cleanup that must finish after cancellation (deleting the `kind` cluster, removing the temp worktree).
- Long loops (`for _, target := range targets`) check `ctx.Err()` at the top of each iteration.
- Use `time.Duration` everywhere; config values in minutes are converted at load time, once.

