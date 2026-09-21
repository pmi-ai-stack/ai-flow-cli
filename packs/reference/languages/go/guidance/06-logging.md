## 6. Logging and the two streams

- **stdout = the envelope. stderr = everything else.** There is no third option. **[review]**
- `log/slog`, configured **once** in `PersistentPreRunE`: `TextHandler` to stderr, level `Info`; `--verbose` → `Debug`. No JSON logs (the envelope is the machine output).
- Packages receive a `*slog.Logger` through their constructor (`New(cfg Config, x Exec, log *slog.Logger)`); they never call `slog.Default()`, `slog.SetDefault()` or the package-level `slog.Info`. **[review]**
- Key/value pairs, not `Sprintf` messages: `log.Debug("exec", "tool", tool, "args", args, "dir", dir)`. Use the key `"err"` for errors.
- Levels: `Debug` = provider/tool invocations, `Info` = one line per stage step, `Warn` = something ignored (goes into `warnings` too), `Error` = only in `main`.
- **Every handler writes through `redact.Writer`** (§8); logging is the most common secret leak.
- Never log the value of a config field whose name matches `token`, `password`, `secret`, `key`, `kubeconfig` — `doctor` reports *presence*, never values.

