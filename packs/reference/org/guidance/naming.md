## Naming

1. Names say what a thing *is* or *does*, not how it's implemented — `RetryPolicy`, not `ExponentialBackoffConfig` unless the backoff shape genuinely is the contract.
2. One vocabulary per concept across the whole org: pick "ticket" or "issue," "repository" or "repo," and use it everywhere — READMEs, code, PRs, commit messages, dashboards. A glossary file per repository beats rediscovering synonyms in review.
3. Booleans read as an assertion: `is_active`, `has_expired`, `can_retry` — never `status` for a two-state field.
4. Abbreviate only industry-standard terms (`ID`, `URL`, `API`); everything else is spelled out in a public API, a config key or a metric name.
5. Services, repositories and packages: `<domain>-<capability>` (`billing-invoicing`, not `invoicing2` or `new-billing`). Never encode a team name that will move.
6. Environment and resource names carry tier and region when it's not otherwise obvious from context (`prod-eu-billing-db`), and nothing else — no dates, no ticket numbers, no owner initials.
7. A rename is a `must`-guidance violation to skip silently: update every reference in the same change, including dashboards and runbooks outside the repository if you can reach them.
