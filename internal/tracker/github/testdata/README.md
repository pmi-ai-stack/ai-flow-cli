Recorded from the live GitHub REST API (unauthenticated, public repository
`cli/cli`) on 2026-10-10, unchanged:

- `issue-bug.json` — `GET /repos/cli/cli/issues/14389` (open; labels `bug`, `priority-2`; 8 comments)
- `comments-page1.json` — `GET /repos/cli/cli/issues/14389/comments?per_page=100&page=1`
- `issue-is-pr.json` — `GET /repos/cli/cli/issues/14636` (a pull request; the issues API returns it with `pull_request`)
- `labels-page1.json` — `GET /repos/cli/cli/labels?per_page=100&page=1`
- `error-401.json` — `GET /user` without a token
- `error-404.json` — `GET /repos/cli/cli/issues/99999999`

Derived with `jq` from the recordings above, because producing them for real
needs write access to the repository:

- `issue-in-progress.json` / `issue-in-review.json` — `issue-bug.json` plus an `in progress` / `in review` label
- `comments-with-marker.json` — `comments-page1.json` with `<!-- ai-flow:impact -->\n` prepended to the 4th comment's body
- `comments-empty.json` — `[]`, the empty page that ends pagination
