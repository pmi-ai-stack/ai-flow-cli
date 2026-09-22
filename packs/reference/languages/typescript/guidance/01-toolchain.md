## Toolchain

- Node LTS, pinned via `.nvmrc`/`engines` in `package.json`. `package-lock.json` (or the project's chosen lockfile) is always committed and never hand-edited.
- `tsc --noEmit` in CI as the type-check gate, independent of whatever bundler/transpiler ships the runtime build.
- One test runner per repository (`jest`, `vitest`, or `playwright test` for e2e) — not two competing ones in the same package.
