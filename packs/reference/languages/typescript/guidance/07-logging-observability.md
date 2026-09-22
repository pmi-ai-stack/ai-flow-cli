## Logging and observability

- A structured logger (`pino`, `winston`, or the platform's own wrapper) — never bare `console.log` in service code (a CLI's own user-facing stdout is a different, deliberate channel, not a log).
- Every request handler logs entry (method, path, correlation id) and exit (status, duration) at `info`; anything at `debug` is stripped or gated in production builds.
- Frontend: user-impacting errors are reported to the error-tracking service (Sentry or equivalent) with enough context to reproduce — component name, props relevant to the failure, not the full Redux store.
- Never log a raw request/response body that might carry a credential, token or full PII record — log the fields actually needed, explicitly.
