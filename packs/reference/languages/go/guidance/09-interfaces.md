## 9. Interfaces, dependency injection, addons

- **Accept interfaces, return structs. Define the interface where it is consumed.** `internal/ops/risk` does not import `exec`; `internal/standards/runners/lint` declares the two-method `runner` it needs. **[review]**
- The spec's interfaces (`Tracker`, `Host`, `MutationTool`, `Addon`, `Exec`, `Resolver`, `CheckRunner`) are the *only* ones defined next to their implementations, because they are the plugin points. Keep them exactly as the spec signs them; a change is a spec change first.
- Every implementation carries a compile-time check: `var _ ops.Addon = (*Terraform)(nil)`.
- **No interface with one implementation and no test that needs it.** Start with the concrete type.
- **Registries are explicit tables, not `init()` side effects:** `internal/ops/registry.go` has `func Addons() []Addon { return []Addon{terraform.New(...), kubectl.New(...), …} }`. Same for mutation tools and check runners. A test asserts each registered name has a matching `skills/ops-addons/<name>/` directory in the plugin repo (path from an env var, skipped when unset).
- Constructors: `func New(cfg Config, x Exec, log *slog.Logger) *Client` — dependencies in, ready-to-use value out. Options struct for many optional knobs; no functional options (this is not a public library).
- **No mutable package-level state.** Constants, sentinel errors, allowlist tables and embedded assets only. **[tool: `gochecknoglobals` with the allowlist file excluded]**
- Addons see `Repo`, `Target`, `Env`, `Rendered` and return the normalized types; they never read `config.json` directly (they get their `OpsConfig` slice) and never write to the handoff dir (the stage code does, through `handoff`).
- Pointer receivers for anything holding an `Exec`, a logger or a mutex; be consistent within a type. Don't embed types in exported structs.

