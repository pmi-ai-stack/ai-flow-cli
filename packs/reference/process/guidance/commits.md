## Commits

1. Conventional Commits shape: `<type>(<scope>): <subject>` — `type` ∈ `feat|fix|refactor|test|docs|chore|perf|build|ci`, `scope` the affected package or area, `subject` imperative mood, no trailing period, under 72 characters. `standards check --stage ship`'s `commit` kind enforces exactly this against `commit-message.md`.
2. The ticket key appears in the commit — either as a trailer (`Refs: PROJ-123`) or in the branch it merges from; never fabricate a key.
3. A commit is one coherent change. If the subject needs "and" to describe it, it's probably two commits.
4. The body (when there is one) explains *why*, not *what* — the diff already shows what changed; nobody rereading history in six months needs that repeated in prose.
5. Never commit generated artifacts, secrets, or anything `.gitignore` already excludes, even "just this once" to unblock a build.
6. A commit that only reformats or renames is its own commit, separate from any behavioural change, so review and `git blame` both stay useful.
