## API design

1. A public API's shape is the contract, not the implementation behind it — design it from the caller's side first, sketch the request/response before writing the handler.
2. Additive, backward-compatible changes only on an existing version: new optional fields, new endpoints. A breaking change is a new version, with both served through a documented deprecation window.
3. Errors follow one consistent shape across every endpoint in a service — the same envelope, the same field names for code/message/details — so a client writes one error handler, not one per endpoint.
4. Pagination, filtering and sorting use the same parameter names and semantics across every list endpoint in the org, not per-team conventions.
5. Idempotency for anything that can be safely retried: an idempotency key on writes that might be resubmitted by a flaky client or an at-least-once queue.
6. Document the contract where the client will actually look — generated from the schema/types when possible, so the doc can't drift from the code silently.
