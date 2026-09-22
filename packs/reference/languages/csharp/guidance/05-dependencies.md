## Dependencies

- Central Package Management (`Directory.Packages.props`) pins every NuGet package version once for the whole solution — no per-project version drift.
- `dotnet list package --vulnerable` (or equivalent) in CI; a `critical`/`high` advisory with a fix available blocks merge.
- License review for any new package outside the org's approved set (`dependencies.json`'s own `allowed_licenses`), same as every other language here.
- Prefer BCL/framework primitives (`System.Text.Json`, `System.Threading.Channels`) over a third-party package that duplicates them, unless there's a concrete, stated reason.
