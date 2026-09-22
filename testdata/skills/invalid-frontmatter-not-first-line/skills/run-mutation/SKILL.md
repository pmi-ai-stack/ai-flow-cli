
---
name: run-mutation
description: Runs the mutation-testing stage for an ai-flow ticket and decides whether survivors are equivalent or need a new test. Loaded by the develop orchestrator at the mutation stage; not for direct use.
license: MIT
compatibility: Requires the ai-flow CLI on PATH and a git checkout of the in-scope repositories.
---

# Run mutation

## CLI calls

Run `ai-flow stage start KEY mutation`, then `ai-flow mutation scope KEY`, then `ai-flow mutation run KEY`, then `ai-flow mutation summarize KEY --accept --reason "a false positive, confirmed with the addon skill"`, and finally `ai-flow stage complete KEY mutation`.

## Read first

1. `mutation-results.json` for the current score.
2. `references/tdd.md` for how a new test must be written.

## Workflow

- [ ] Run `ai-flow mutation run KEY`
- [ ] Run `ai-flow mutation summarize KEY`
- [ ] Run `ai-flow stage complete KEY mutation`

## Judgement

- A surviving mutant is equivalent only with a written reason; otherwise, add a killing test.

## Stop and ask

- The score stays below threshold after every round and no reason justifies accepting it.

## Done when

`stage complete` succeeded and every survivor is explained.
