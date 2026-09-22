## Logging and observability

- `ILogger<T>` injected, structured message templates (`_logger.LogInformation("Order {OrderId} processed in {Duration}ms", orderId, duration)`) — never string-interpolated messages that lose the structured fields.
- Log levels follow the framework's own semantics: `Critical`/`Error` for things needing attention, `Warning` for degraded-but-handled, `Information` for normal lifecycle events, `Debug`/`Trace` off in production by default.
- `System.Diagnostics.Activity`/OpenTelemetry for tracing across async boundaries and service calls — don't hand-roll correlation ids when the platform already propagates them.
- Never log a full exception's `Message` when it might carry a connection string or credential — log the exception type and a redacted summary, and rely on structured exception logging (`_logger.LogError(ex, "...")`) which already separates the stack trace from the templated message.
