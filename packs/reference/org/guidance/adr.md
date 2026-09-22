## Architecture decision records

1. Write an ADR when a change picks between real alternatives that a future reader would otherwise have to reverse-engineer from the code: a new datastore, a cross-service protocol, a major dependency, a reversal of an earlier decision.
2. One ADR per decision, numbered and immutable once accepted — a later change that revisits it writes a new ADR that supersedes the old one, never edits it in place.
3. Shape: Context (the problem, the forces in tension), Decision (what was chosen, one sentence), Consequences (what this makes easier, what it makes harder, what it forecloses), Alternatives considered (and why not).
4. An ADR is short — half a page is normal, two pages is a sign the decision itself is still unclear, not that the ADR needs more words.
5. `standards check`'s `docs` kind expects an ADR to exist when `plan.md` marks the change architectural — mark it, or explain in the plan why the change looks architectural but isn't.
