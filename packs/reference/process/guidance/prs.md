## Pull requests

1. Required sections in the body: Summary (what and why), Changes (bullet list), Test plan (what was actually run), Risk/rollback (blank when there's nothing notable, filled in for anything touching data, infra or a public contract). `standards check --stage ship`'s `pr` kind enforces these against `pr-body.md`.
2. Size is a signal, not a hard rule: a PR that's hard to review because it does five unrelated things should be five PRs; a PR that's large because it's one mechanical rename is fine, and says so in Risk/rollback.
3. Link the ticket. A PR with no ticket reference is a PR reviewers can't get context for without asking.
4. Draft PRs are for work in progress you want early feedback on — mark them draft (`host.draft_prs`), don't request review on unfinished work and call it "still WIP" in a comment instead.
5. The generated summary block (score, checks, mutation, plan link) is part of the PR body — never edit or remove it by hand.
6. Merge only after the required checks are green and the required approvals are in — no "I'll fix the failing check after merge."
