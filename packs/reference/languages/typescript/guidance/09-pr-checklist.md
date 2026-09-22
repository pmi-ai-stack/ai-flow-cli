## PR checklist

- [ ] `tsc --noEmit` clean, no new `any`/`@ts-ignore` without a comment.
- [ ] `eslint`/`prettier` clean, no inline-disabled rule without a reason.
- [ ] New/changed behaviour has a RED-first test; existing tests weren't edited just to make them pass again.
- [ ] No `console.log` left from debugging.
- [ ] Public exports (types, functions, components) have a doc comment if their purpose isn't obvious from the name.
- [ ] Dependency changes are justified in the PR body per `05-dependencies.md`.
