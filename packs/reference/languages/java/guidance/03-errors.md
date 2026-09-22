## Errors

1. A small hierarchy of unchecked, domain-specific exceptions (`class OrderNotFoundException extends RuntimeException`) rather than reusing `RuntimeException`/`IllegalStateException` directly for conditions a caller needs to distinguish.
2. Checked exceptions are reserved for conditions a caller is genuinely expected to recover from inline — most new APIs should not introduce a new checked exception type.
3. Never `catch (Exception e) {}` — an empty catch block is never acceptable, not even temporarily.
4. Preserve the cause when wrapping: `throw new ServiceException("...", e)`, never discard the original exception.
5. Validate arguments at a public method's entry (`Objects.requireNonNull`, an explicit `IllegalArgumentException`) — fail at the boundary, not several calls deeper with a confusing `NullPointerException`.
6. Log an exception exactly once, at the point it's finally handled (converted to a response, retried, or given up on) — not once per layer it passes through on the way up.
