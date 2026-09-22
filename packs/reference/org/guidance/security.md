## Security

1. Secrets live in the secret manager or environment variables injected by it — never in a repository, a config file, a log line, a comment or a ticket. A secret that reaches a diff is a `must` finding with no waiver (`secrets` checks cannot be waived — see the pack format's own loader rule).
2. Every external input is validated at the boundary it enters: request bodies, webhook payloads, file uploads, CLI arguments. Validate shape and range before any business logic touches it.
3. Least privilege by default: a new service account, IAM role or kube `ServiceAccount` gets exactly the verbs and resources it needs today, not "admin, we'll tighten it later."
4. Dependencies are pinned and scanned; a known-vulnerable version blocking a release is expected behaviour, not a false positive to suppress.
5. Authentication and authorization are never rolled by hand when a maintained library or platform primitive exists for it.
6. A security-relevant change (auth, secrets handling, permissions, data access) gets an explicit callout in the PR description, even when the diff itself looks small.
