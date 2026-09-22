## Definition of done

A ticket is done when, and only when:

1. Every `must` guidance section applies and is satisfied, or waived with a reason and an expiry.
2. Tests are green, written RED-first for the new behaviour, and mutation-adequate (at or above threshold, or below with a written accept reason).
3. `standards check`'s `must` findings are all fixed or waived — none left `open`.
4. The PR body's required sections are filled in for real, not templated placeholders.
5. Anything the change makes true in production has monitoring or an alert to notice if it stops being true.
6. Docs, runbooks and ADRs affected by the change are updated in the same change — not a follow-up ticket that may never get picked up.
7. For an OPS change: the rollback plan names every non-revertible action and its mitigation, and the risk score has been reviewed against the blast radius, not just the number.

"It works on my machine" and "the tests I happened to run passed" are not on this list on purpose.
