## Logging and observability

- SLF4J as the logging facade (Logback or Log4j2 as the implementation) — never `System.out.println` in service code, and never a direct dependency on the logging implementation from application code.
- Parameterized logging (`log.info("order {} processed in {}ms", orderId, duration)`), never string concatenation — it avoids the formatting cost when the level is disabled and keeps fields structured for log aggregation.
- Log levels follow SLF4J's own semantics: `ERROR` needs attention, `WARN` is degraded-but-handled, `INFO` is a normal lifecycle event, `DEBUG`/`TRACE` off in production by default.
- Micrometer (or the platform's own metrics library) for the golden signals; don't hand-roll counters when the framework already exposes them via `/actuator/metrics` or equivalent.
- Never log a full request/response object that might carry a credential or full PII record — log the specific fields actually needed.
