## Branching

1. One branch per ticket, named `<type>/<KEY>` (`feature/PROJ-123`, `bugfix/PROJ-124`) — `ai-flow`'s own `ticket fetch` derives this automatically from the tracker's issue type against `tracker.bug_types`.
2. Branch from the repository's configured default base branch, kept current with it — a stale base is a `base_diverged` refusal at `git prepare`, not something to work around with a manual merge mid-ticket.
3. No direct commits to the default branch. Every change, however small, goes through a PR.
4. A branch lives as long as its ticket does. Once the PR merges, delete it — a long-lived branch drifting from main is a merge conflict waiting to happen, not a safety net.
5. Never rebase a branch another person has already pulled without agreeing on it first; force-pushing a shared branch rewrites history out from under them.
