## Errors

1. Custom exception types for domain failures a caller needs to distinguish (`OrderNotFoundException : DomainException`); the built-in `Exception` type itself is never thrown directly.
2. `catch` only what you can act on. A `catch (Exception)` at the outermost boundary (a middleware, a top-level handler) logs and converts to a stable response — it never appears mid-stack "just in case."
3. Never use exceptions for expected control flow (e.g. `TryParse`/`TryGetValue` patterns exist precisely so a common failure isn't an exception at all).
4. Preserve the original exception when wrapping: `throw new OrderProcessingException("...", innerException: ex)`, never a bare `throw new X(ex.Message)` that discards the stack trace.
5. `throw;` (bare) to rethrow and preserve the original stack trace — never `throw ex;`, which resets it.
6. Validate arguments at a public method's entry with `ArgumentNullException.ThrowIfNull`/`ArgumentException` — fail at the boundary, not three calls deeper with a confusing `NullReferenceException`.
