## Layout

```
src/
  <feature>/
    <feature>.ts
    <feature>.test.ts
    index.ts        # re-exports the feature's public surface only
  lib/               # cross-feature helpers with no feature of their own
index.ts             # the package's own public surface
```

- A test lives next to the file it tests (`foo.ts` / `foo.test.ts`), not in a parallel `__tests__/` tree that drifts from the source layout.
- `index.ts` re-exports; it contains no logic of its own. A feature's internals are not imported directly from outside the feature directory.
- Barrel files (`export * from "./x"`) are for a feature's own public surface only — never a project-wide barrel that forces every consumer to pull in everything.
