---
name: addon-example-tool
description: Tool-specific judgement for the example-tool addon, in the five fixed sections every addon skill carries. Loaded by OPS stage skills for example-tool targets; not for direct use.
license: MIT
compatibility: Requires the ai-flow CLI on PATH and a git checkout of the in-scope repositories.
---

# Addon: example-tool

## Drift

Expected drift for this tool is limited to a small set of controller-managed fields; anything else is real drift.

## Changeset

A change to a selector or a storage class is dangerous and should replace the target; a label-only change is cosmetic.

## Impact

Blast radius grows fastest through a shared module; splitting the change into smaller, independent units shrinks it.

## Testing

See `references/tdd.md` for the minimal test shape this tool's own harness expects.
