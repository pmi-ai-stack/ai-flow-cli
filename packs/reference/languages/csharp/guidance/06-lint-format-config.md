## Lint and format configuration

- `.editorconfig` at the solution root is the one source of style truth — naming conventions, `var` usage, brace style, analyzer severities — enforced by `dotnet format --verify-no-changes` in CI, the `format` kind's own tool for this language.
- Roslyn analyzers (`Microsoft.CodeAnalysis.NetAnalyzers`, nullable-aware) run as part of the normal build, warnings-as-errors for anything above the config's stated severity floor.
- A `#pragma warning disable` carries the specific warning code and a reason comment, and is re-enabled immediately after the line(s) it covers — never left disabled for the rest of the file.
- StyleCop (or the analyzer set a repository has chosen) config lives in `.editorconfig` alongside everything else — not a second, competing config file.
