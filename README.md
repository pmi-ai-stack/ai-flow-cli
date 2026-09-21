# ai-flow CLI

The deterministic side of the ai-flow pipeline. One Go binary; every tracker, git, git-host, infra and test-tool side effect goes through it. The agent (Claude Code / Cursor skills in the sibling [`ai-flow`](../ai-flow) plugin repo) carries judgement only and talks to this binary exclusively through `ai-flow <noun> <verb> --json`.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/pmi-ai-stack/ai-flow-cli/main/install.sh | sh
```

Or download a release archive from the [releases page](https://github.com/pmi-ai-stack/ai-flow-cli/releases) and verify it against `checksums.txt` yourself.

## The contract

- Every command prints exactly one JSON envelope on stdout (`{ok, command, result, needs_input, error, warnings}`); progress goes to stderr. `--human` renders it for people; `--json` is the default.
- Exit codes 0–6 carry a documented `error.code`; exit 4 always means `needs_input` — the CLI never asks a question itself.
- Nothing is committed, pushed, transitioned or applied unless `status.json` records the gate that authorizes it.

See [`AGENTS.md`](AGENTS.md) for the packages that implement this contract and how this repo is organized.

## Build from source

```sh
go build -o dist/ai-flow ./cmd/ai-flow
```

Requires Go 1.27+. See `packs/reference/languages/go/guidance/` for the coding rules and `AGENTS.md` for the pre-PR command.
