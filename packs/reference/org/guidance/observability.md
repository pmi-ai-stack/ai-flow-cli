## Observability

1. Every service exposes the four golden signals for its own traffic: latency, traffic, errors, saturation. A new endpoint that skips this is not done.
2. Metrics are for aggregates and alerting; logs are for a single event's detail; traces are for "why was this one request slow." Reach for the cheapest one that answers the question — don't trace what a counter already tells you.
3. An alert fires on symptoms a human can act on (error rate, latency SLO burn), not on causes a human would have to go dig for (CPU at 80%). Cause-level metrics still exist — as dashboards, not pages.
4. Every alert links to a runbook. An alert with no runbook is a future 3am mystery, not a safety net.
5. Dashboards default to the SLO window the service is actually held to, not "last hour" for a service with a 30-day SLO.
6. New infrastructure ships with monitoring in the same change — never a stated follow-up ticket.
