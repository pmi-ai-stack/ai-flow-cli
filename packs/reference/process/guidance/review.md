## Review

1. Review for correctness first, style last — a linter already caught the style issues that matter; a human's time goes to "does this do what it claims, and what does it miss."
2. Every `must`-severity finding gets a citation to the section id it violates (`go.errors`, `org.security`, …) so the author knows exactly which rule and can propose a waiver if they genuinely disagree, rather than guessing at intent.
3. Distinguish a blocking finding from a suggestion explicitly — "must fix" vs. "consider" — so an author isn't left guessing which comments gate the merge.
4. Review the tests as carefully as the code: a green suite that doesn't exercise the actual change is not coverage, it's decoration.
5. A reviewer who can't finish in one sitting says so and picks it back up, rather than rubber-stamping to clear the queue.
6. Disagreements that don't resolve in the thread move to a synchronous conversation — a PR comment war helps no one, least of all whoever reads the thread later.
