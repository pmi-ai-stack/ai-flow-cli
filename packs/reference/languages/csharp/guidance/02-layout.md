## Layout

```
src/<Solution>.<Project>/
  <Project>.csproj
  <Area>/<Thing>.cs
tests/<Solution>.<Project>.Tests/
  <Area>/<Thing>Tests.cs
```

- One namespace per folder, matching the folder path from the project root (`<Solution>.<Project>.<Area>`) — file-scoped namespaces (`namespace Foo.Bar;`), not the nested-brace form.
- A test project mirrors its subject project's folder structure one level in, named `<Project>.Tests`.
- Interfaces defined next to their primary implementation unless genuinely shared across several implementations, in which case they move to a `Contracts`/`Abstractions` project.
