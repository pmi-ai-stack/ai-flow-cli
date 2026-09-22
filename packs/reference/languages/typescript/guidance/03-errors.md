## Errors

1. `throw` only `Error` subclasses, never a string or a plain object — `instanceof` checks and stack traces both depend on it.
2. Define a small hierarchy of typed errors for a module's own failure modes (`ValidationError`, `NotFoundError`, `ConflictError`) rather than one generic `Error` with a `code` string property nothing checks consistently.
3. `catch` blocks either handle the error meaningfully or rethrow — an empty `catch {}` is never acceptable, not even "just for now."
4. Async errors: a rejected `Promise` an `await` doesn't catch propagates to the nearest `try`/`catch` or the caller — never let one vanish into an unhandled-rejection warning.
5. At an API boundary, map internal error types to a stable, documented response shape — never leak an internal error's message or stack to an external caller.
6. `Error.cause` (or an explicit `cause` field on a custom error) when wrapping a lower-level error, so the chain survives past the boundary that wrapped it.
