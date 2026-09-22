## Logging and observability

- The stdlib `logging` module (or `structlog` for structured output), configured once at the application entry point — never a per-module `print()` for anything meant to survive past local debugging.
- Loggers are named per module (`logging.getLogger(__name__)`), never a single global logger that makes it impossible to filter by source.
- Structured fields (`logger.info("invoice processed", extra={"invoice_id": ..., "amount": ...})`) over an interpolated message a downstream parser would have to regex apart.
- No secret, token or full unredacted identifier in a log call — the same rule `org/guidance/security.md` states applies here without exception.
