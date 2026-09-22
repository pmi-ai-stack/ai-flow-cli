## PR checklist

- [ ] `ruff check`/`ruff format --check` clean.
- [ ] `mypy`/`pyright` clean on every touched module.
- [ ] New/changed behaviour has a RED-first `pytest` test; existing tests weren't edited just to make them pass again.
- [ ] No leftover `print()` debugging statements.
- [ ] Public functions and classes have type hints and a docstring if the purpose isn't obvious from the name.
- [ ] Dependency changes are justified in the PR body per `05-dependencies.md`.
