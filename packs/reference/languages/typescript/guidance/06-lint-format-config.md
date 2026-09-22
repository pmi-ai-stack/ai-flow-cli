## Lint and format configuration

- `eslint.config.js` (flat config) is the one source of lint truth; a rule disabled inline (`// eslint-disable-next-line`) carries a comment saying why, and is reviewed as carefully as the code it's guarding.
- `prettier --check` gates CI; `prettier --write` is the only accepted way to fix a formatting failure — never hand-format to satisfy the checker.
- `@typescript-eslint/recommended-type-checked` (or stricter) as the base rule set — the type-aware rules catch what plain ESLint structurally cannot.
- `.editorconfig` fixes indentation, line endings and final-newline across every editor on the team, independent of what any one person's IDE defaults to.
- A repository-specific rule override lives in the repository's own `.ai-flow/standards/` override, with a reason — never a silent divergence from the pack's shipped config.
