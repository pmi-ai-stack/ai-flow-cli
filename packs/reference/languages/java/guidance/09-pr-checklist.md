## PR checklist

- [ ] `checkstyle` clean, no new inline suppression without a reason.
- [ ] Static analyzer (SpotBugs/Error Prone) clean on touched code.
- [ ] New/changed behaviour has a RED-first `JUnit 5` test; existing tests weren't edited just to make them pass again.
- [ ] No leftover `System.out.println`/`e.printStackTrace()` debugging calls.
- [ ] Public classes and methods have Javadoc if the purpose isn't obvious from the name.
- [ ] Dependency changes are justified in the PR body per `05-dependencies.md`.
