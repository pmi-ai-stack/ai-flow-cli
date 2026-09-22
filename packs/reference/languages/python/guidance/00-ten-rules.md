## Python: ten rules

1. Type hints on every public function signature; `mypy`/`pyright` in strict mode gates CI, not just an editor hint nobody enforces.
2. `ruff` for lint and format both — one tool, one config, no `black`+`flake8`+`isort` triangulation to keep in sync.
3. No bare `except:` — catch the specific exception type you can actually handle; let everything else propagate.
4. Dataclasses (`@dataclass`, `attrs`, or `pydantic` models at a boundary) instead of a `dict` passed around as an implicit schema.
5. `pathlib.Path`, never raw string concatenation for filesystem paths.
6. Dependency and interpreter versions pinned via `pyproject.toml` + a lockfile (`uv.lock`, `poetry.lock`); never a bare `pip install` with no lock.
7. Mutable default arguments are a bug, not a style nit (`def f(x, items=[])`) — use `None` and initialize inside.
8. Context managers (`with`) for anything with a resource to release — files, locks, connections, temp directories.
9. f-strings for interpolation; `%`-formatting and `.format()` are legacy, kept only in code you're not otherwise touching.
10. `__init__.py` files define a package's public surface via `__all__`; internal modules are prefixed `_` and never imported from outside the package.
