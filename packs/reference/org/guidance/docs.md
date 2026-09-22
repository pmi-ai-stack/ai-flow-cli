## Docs

1. Every repository has an `AGENTS.md` (or `CLAUDE.md`) naming what the repository is, how to build/test it, and any rule specific to it that guidance here doesn't already cover — the file `standards`'s repo layer injects verbatim as `repo.instructions`.
2. A README answers "what is this and how do I run it" in under a minute of reading — architecture deep-dives, ADRs and runbooks live elsewhere and are linked, not inlined.
3. Public functions, exported types and API endpoints carry a doc comment stating what they do and any non-obvious constraint (nil handling, ordering, units) — not what the name already says.
4. A doc that references a specific file, function or command is wrong the moment that thing is renamed. Prefer linking to a stable concept over pasting a path that will drift.
5. Runbooks are written for the person paged at 3am who has never touched this service: exact commands, exact dashboard links, no "you'll know it when you see it."
6. Delete docs for things that no longer exist in the same change that removes the thing — a stale doc is worse than no doc.
