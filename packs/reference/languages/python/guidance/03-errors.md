## Errors

1. Define a small exception hierarchy per module (`class BillingError(Exception)`, then specific subclasses) rather than raising the stdlib's generic `Exception`/`ValueError` for domain failures a caller needs to distinguish.
2. Catch the narrowest type you can actually handle. A bare `except Exception` at a module boundary (a request handler, a CLI entry point) is the one acceptable place — and it logs and re-raises or converts, never silently swallows.
3. `raise ... from err` when wrapping a lower-level exception, so the chain survives in the traceback instead of hiding the original cause.
4. Never use exceptions for ordinary control flow (e.g. probing "does this exist" with a `try`/`except` where a check would do) unless the check itself would introduce a race.
5. An `assert` is a debugging aid stripped under `-O` — never use it to validate user input or enforce a runtime invariant that must hold in production.
