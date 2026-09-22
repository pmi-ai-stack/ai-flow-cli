## Toolchain

- .NET SDK version pinned via `global.json`; CI installs exactly that version, never "whatever's latest on the runner image."
- `dotnet test` as the one test entry point, whichever framework (`xUnit` preferred for new projects) a project uses underneath.
- `Directory.Build.props`/`Directory.Packages.props` for solution-wide settings (nullable, analyzers, central package versions) instead of repeating them per `.csproj`.
