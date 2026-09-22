## Logging

1. Structured, always — key-value or JSON, never an interpolated sentence a parser has to regex apart.
2. One log line per meaningful event, not one per function call. A request gets an entry line and an exit line with duration and outcome, not a line per internal step.
3. Levels mean something specific: `error` = this needs a human, now or soon; `warn` = degraded but self-healing or already retried; `info` = a normal event worth a permanent record; `debug` = off in production by default, on for one investigation at a time.
4. Every line carries a correlation id (request id, ticket key, trace id) so a single flow's lines can be pulled back together across services.
5. Never log a secret, a full credential, or an unredacted customer identifier beyond what the retention policy allows — see `security.md` for what "redacted" means here.
6. Log the decision, not just the data: "skipping retry: circuit open for payments-api" tells a reader more than the circuit-breaker's internal counters ever will.
