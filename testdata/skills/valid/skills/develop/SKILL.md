---
name: develop
description: Drives an ai-flow ticket through every stage from fetch to output, relaying gates and needs_input to the developer. Use when the developer wants to advance a ticket to its next stage.
license: MIT
compatibility: Requires the ai-flow CLI on PATH and a git checkout of the in-scope repositories.
disable-model-invocation: true
---

# Develop

## CLI calls

Run `ai-flow status KEY` to find what happens next. When the result names a CLI-only stage rather than a skill, run the very instruction it names — for example `ai-flow ticket fetch KEY` or `ai-flow repo locate KEY` — then check status again.

## Read first

1. `ai-flow status KEY`'s own result.

## Workflow

- [ ] Run `ai-flow status KEY`
- [ ] Load the skill it names, or run the CLI-only instruction directly
- [ ] Repeat until the result names none

## Judgement

- Present every gate before approving it, and relay `needs_input`'s question verbatim.

## Stop and ask

- `needs_input` is present in the result.

## Done when

`output.md` exists and the developer has been told the ticket is ready to ship.
