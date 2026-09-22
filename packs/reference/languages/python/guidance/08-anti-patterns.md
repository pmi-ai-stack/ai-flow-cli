## Anti-patterns

- Mutable default arguments (`def f(items=[])`) — the list is shared across every call that doesn't pass its own.
- `from module import *` outside a deliberate, narrow `__init__.py` re-export — it hides where a name actually comes from.
- Catching `Exception` broadly and logging-then-continuing as if the failure were handled, when it wasn't.
- Using a `dict` as an ad hoc struct passed through several layers instead of a `dataclass`/`pydantic` model with an actual schema.
- Circular imports papered over with a late `import` inside a function body — the cycle is a structure problem to fix, not to hide.
- Comparing to `None` with `==` instead of `is` — works today by accident, breaks the moment a `__eq__` override changes the semantics.
