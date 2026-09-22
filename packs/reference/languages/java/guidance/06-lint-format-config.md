## Lint and format configuration

- `checkstyle.xml` is the one source of style truth, run in CI as a build-failing step, not an advisory report nobody reads.
- A rule suppression (`@SuppressWarnings`, a checkstyle `// CHECKSTYLE:OFF`) carries the specific rule and a reason, scoped as narrowly as possible and re-enabled immediately after.
- A static analyzer (SpotBugs/Error Prone) runs alongside checkstyle — checkstyle catches style, these catch actual bug patterns (resource leaks, incorrect equals/hashCode, null-handling).
- `.editorconfig` fixes indentation and line endings across editors, independent of checkstyle's own structural rules.
