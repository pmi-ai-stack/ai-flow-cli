## Errors

1. An error a caller might reasonably branch on is a typed or sentinel value, documented at the point it's returned — never a string the caller has to substring-match.
2. Every error that crosses a service boundary carries enough context to act on without re-reading the code: what was being attempted, which identifier, why it's unusual. "invalid input" is not an error message; "invoice 4821: currency \"XBT\" is not in the supported set" is.
3. Never swallow an error to keep a function's signature clean. If a caller truly cannot act on it, log it once at the point it's discarded and say why discarding is safe.
4. User-facing and operator-facing errors are different messages. A user sees "we couldn't process your payment, try again in a few minutes"; an operator's log sees the provider's declined-transaction code.
5. Retry only errors that are actually transient (timeouts, `5xx`, rate limits) and only with backoff and a cap. Retrying a `4xx` or a validation failure is a bug, not resilience.
6. An error that repeats — the same cause, many callers — becomes a shared, named condition (a sentinel, an alert, a runbook entry), not five independently-worded copies.
