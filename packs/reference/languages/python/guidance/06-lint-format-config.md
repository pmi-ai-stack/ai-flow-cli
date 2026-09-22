## Lint and format configuration

- `ruff.toml` is the one source of truth for both lint and format — `ruff check` and `ruff format --check` both gate CI.
- `# noqa` carries the specific rule code and a reason (`# noqa: E501 — long URL, not splittable`), never a bare blanket suppression.
- `mypy`/`pyright` strict mode; a module not yet type-clean is listed explicitly in the config's per-module overrides with a reason, not silently excluded.
- `.editorconfig` fixes indentation and line endings across editors, independent of `ruff format`'s own style rules.
