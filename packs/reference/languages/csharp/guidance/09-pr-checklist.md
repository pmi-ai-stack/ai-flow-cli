## PR checklist

- [ ] `dotnet format --verify-no-changes` clean.
- [ ] `dotnet build` clean with analyzers on, no new suppressed warnings without a reason.
- [ ] New/changed behaviour has a RED-first `xUnit` test; existing tests weren't edited just to make them pass again.
- [ ] No `.Result`/`.Wait()` introduced on an async call.
- [ ] Public types and members have XML doc comments if the purpose isn't obvious from the name.
- [ ] Dependency/package changes are justified in the PR body per `05-dependencies.md`.
