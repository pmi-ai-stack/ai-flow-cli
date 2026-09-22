## Dependencies

- A new dependency is justified in the PR: what it replaces (hand-rolled code, a heavier alternative), and its maintenance signal (recent releases, open issues, license).
- No dependency with a license outside the org's approved set (`dependencies.json`'s own `allowed_licenses`).
- `npm audit`/equivalent runs in CI; a `critical`/`high` advisory with a fix available blocks merge, not just a warning.
- Pin exact versions for anything security-sensitive (auth, crypto); caret ranges are fine for the rest, resolved by the committed lockfile either way.
- Prefer the platform/standard-library primitive over a dependency for anything trivial (`Array.prototype.at`, `structuredClone`) — every dependency is a supply-chain surface, not a free abstraction.
