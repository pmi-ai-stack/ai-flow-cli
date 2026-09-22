## Toolchain

- One supported CPython version per repository, pinned in `pyproject.toml`'s `requires-python` and CI's own runner version — not a range that quietly drifts to whatever's latest.
- `uv` (or `poetry`) for dependency management; `requirements.txt` alone, hand-edited, is not an accepted lockfile.
- `pytest` is the one test runner; `unittest`-style classes are fine to keep if inherited, never newly written.
