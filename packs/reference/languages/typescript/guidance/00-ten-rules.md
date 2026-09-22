## TypeScript: ten rules

1. `strict: true` in `tsconfig.json`, no exceptions. `any` is a documented last resort with a comment saying why, never a shortcut past a type error.
2. Prefer `unknown` over `any` at a boundary (JSON parse, external API response) and narrow it explicitly before use.
3. No `// @ts-ignore` without a linked issue; `// @ts-expect-error` when the suppression is intentional and permanent, so a fixed type actually surfaces as a lint failure.
4. Immutable by default: `const`, `readonly` fields, `ReadonlyArray`/`ReadonlyMap` for anything a caller shouldn't mutate.
5. Discriminated unions over boolean flags for state that only makes sense in combination (`{status: "loading"} | {status: "error", error} | {status: "ready", data}`).
6. Async code is `async`/`await` throughout a call chain — never mix `.then()` chains with `await` in the same function.
7. Every `Promise` is awaited, returned, or explicitly voided (`void somePromise()`) with a comment — a silently dropped rejection is a production incident waiting to happen.
8. Public function signatures are typed explicitly; return-type inference is fine for internal helpers, not for anything another module imports.
9. `eslint`/`prettier` are the style authority; a manual formatting opinion in review that disagrees with the config is a config PR, not a comment.
10. No default exports for anything with more than one thing to export from the module — named exports keep refactors and auto-imports honest.
