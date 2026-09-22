## Layout

```
src/<package>/
  __init__.py
  <module>.py
tests/
  test_<module>.py
pyproject.toml
```

- `src/` layout (not a flat top-level package) so the installed package is what's actually tested, not an accidentally-importable working directory.
- Tests live under `tests/`, mirroring `src/<package>/`'s module structure one level in — `tests/test_billing.py` for `src/<package>/billing.py`.
- A module that grows past a few hundred lines and covers more than one responsibility splits into a subpackage, not a longer single file.
