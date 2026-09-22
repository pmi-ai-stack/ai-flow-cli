## Dependencies

- Every dependency declared in `pyproject.toml`, resolved via the committed lockfile — never a package installed ad hoc into a developer's environment and assumed present elsewhere.
- License and vulnerability scanning (`pip-audit` or equivalent) in CI; a `critical`/`high` advisory with a fix available blocks merge.
- Prefer the standard library over a dependency for anything it already covers well (`dataclasses`, `pathlib`, `contextlib`) — evaluate a new dependency against what it actually saves.
- Pin exact versions for security-sensitive packages (auth, crypto, serialization); compatible-release specifiers (`~=`) are fine for the rest.
