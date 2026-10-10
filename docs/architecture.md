# ai-flow-cli architecture

A map for a new contributor. **The contract lives elsewhere:**
- **The spec:** `../spec-v7/ai-flow-implementation-spec.md`, in the workspace root.
- **This module's code and tests:** see `AGENTS.md`.

This page says where things are and why they are split that way. It restates no rule. Each package's `doc.go` (or its package comment) is the longer story. `TestArchitectureDocNamesEveryPackage` (`cmd/ai-flow/architecture_test.go`) fails when a package under `internal/` is missing from the map below.

## The one idea

The plugin's skills make judgement calls. This binary does everything else: every tracker call, git operation, git-host call, infrastructure tool, test runner and mutation run. Skills call `ai-flow <noun> <verb> --json` and read one JSON envelope from stdout. The CLI never calls a model and never asks a question; ambiguity comes back as exit 4 with `needs_input`.

## How a command runs

1. **Entry:** `cmd/ai-flow/main.go` sets up the signal context, builds the root command and exits with the envelope's code. Nothing else happens there.
2. **Command:** `internal/cli/` holds one file per noun, using cobra. A command:
   - validates its arguments (`TestEveryTicketScopedCommandRejectsMalformedKey`);
   - loads config (`internal/config/`) and the ticket's handoff artifacts (`internal/handoff/`);
   - calls the domain package.
   - The command list itself is data: `handoff.Catalog()` and `handoff.StageTable()` (in `internal/handoff/`) feed `skills lint`, `status` and `explain`.
3. **Domain:** a package such as `internal/standards/`, `internal/ops/`, `internal/gitops/` or `internal/mutation/` does the work. It reaches the outside world only through interfaces (`tracker.Tracker`, `host.Host`, `mutation.Tool`, `ops.Addon`, `exec.Execer`). A new provider is a new package plus a config value. `newTracker` in `internal/cli/tracker.go` is the one place a tracker provider is chosen.
4. **Subprocess:** every subprocess goes through `internal/ops/exec/`, which matches argv against the typed allowlist before running it. It is the only importer of `os/exec` (`TestNoOsExecOutsideExecPackage`). No allowlist row can mutate a persistent infrastructure target (`TestAllowlistNeverMutatesPersistentState`).
5. **Artifacts:** JSON files are written through `handoff.WriteArtifact`: canonical key order, atomic write, schema-validated against `schema/*.schema.json`. The same inputs give the same bytes (`internal/handoff/handofftest/`, `RequireByteStable`).
6. **Output:** `internal/envelope/` prints the one envelope and maps `error.code` to the exit code (`TestExitCodeTable`; `explain` covers every code, `TestExplainEveryKnownCode`). Secrets are removed by `internal/redact/` on every path out: subprocess output, `events.jsonl`, stderr logs and the envelope.

## Package map

### Entry and contract
- `cmd/ai-flow/`: the binary's entry point.
- `internal/cli/`: the cobra commands, one file per noun.
- `internal/envelope/`: the stdout envelope, the exit-code table and the `explain` text.
- `internal/config/`: loads and validates `~/.ai-flow/config.json`, and holds the embedded template (`config init`'s only copy).
- `internal/schema/`: the registry and validator over the embedded schemas in `schema/` (top level, because `go:embed` cannot reach `..`).
- `internal/runlog/`: `--verbose` / `--log-format` diagnostics, to stderr only.
- `internal/redact/`: discovers secrets from the environment and redacts them everywhere.

### Handoff state
- `internal/handoff/`: the home layout, the atomic byte-stable writer, the stage state machine over `status.json`, `active.json`, `events.jsonl`, and the command catalog.
- `internal/handoff/handofftest/`: the byte-stability harness every artifact writer's test uses.
- `internal/render/`: builds `output.md` and the PR body's generated block from a ticket's artifacts.

### Providers
- `internal/tracker/`: the `Tracker` interface.
- `internal/tracker/jira/`: its Jira REST v2 implementation.
- `internal/tracker/github/`: its GitHub Issues implementation. Keys stay `<PREFIX>-<n>` through `tracker.repositories`, and statuses are labels.
- `internal/host/`: the `Host` interface (`pr create`).
- `internal/host/github/`: its GitHub REST implementation.
- `internal/httpretry/`: the request loop both clients share, with per-attempt deadlines, bounded body reads, and retries only where they are safe.

### Repositories and routing
- `internal/gitops/`: every git operation (sync, branch, worktree, diff, commit, push, the merge-base worktree).
- `internal/gitref/`: branch and ref-name validation, shared by config and gitops.
- `internal/routing/`: ticket-to-repository routing precedence, as pure functions.
- `internal/detect/`: probes a repository for its languages, test runner, mutation tool and e2e suite.
- `internal/strutil/`: small string-slice helpers.

### Verification
- `internal/verify/unit/`, `internal/verify/e2e/`: run and parse a repository's unit and e2e suites.
- `internal/verify/visual/`: dev server and screenshot capture for `visual run`.
- `internal/mutation/`: the `Tool` interface and the changed-files scope.
- `internal/mutation/gremlins/`, `internal/mutation/stryker/`, `internal/mutation/mutmut/`, `internal/mutation/pitest/`: one implementation per tool.

### Standards
- `internal/standards/`: the pack format, sync, layering (org/team/repo), resolution, guidance budget and waivers.
- `internal/standards/check/`: runs one resolved check against one repository or target (lint, format, secrets, policy, scan, …) and parses the tool's output into findings.
- `packs/`: embeds the reference pack (`packs/reference/`). Its `languages/go/guidance/` is this module's Go coding rules.

### OPS (infrastructure)
- `internal/ops/`: the shared vocabulary: `Addon`, `Target`, `Changeset`, `Env`, the registry and kind classes.
- `internal/ops/exec/`: the allowlist and the only subprocess runner.
- `internal/ops/exec/exectest/`: scripted test doubles for it.
- `internal/ops/render/`: the real/redacted rendering split, memoized by tree hash.
- `internal/ops/envs/`: binds a target to a live environment.
- `internal/ops/risk/`: the risk-score formula, using integer per-mille.
- `internal/ops/rollback/`: derives the revert path.
- `internal/ops/harness/`: runs infra-test harnesses.
- `internal/ops/addons/terraform/`, `internal/ops/addons/kubectl/`, `internal/ops/addons/helm/`, `internal/ops/addons/kustomize/`, `internal/ops/addons/githubactions/`, `internal/ops/addons/jenkins/`: one package per addon. Each matches the plugin's `addon-<name>` skill.

### Agent hosts and skills
- `internal/agenthost/`: the normalized hook event and decision types, and host detection.
- `internal/agenthost/claudecode/`, `internal/agenthost/cursor/`: each host's payload in and decision out.
- `internal/agenthost/guard/`: the protected table that `hook guard` evaluates, and the guard selftest.
- `internal/skills/`: what a skill directory is.
- `internal/skills/lint/`: `ai-flow/docs/skill-authoring.md`, executable.
- `internal/skills/install/`: copies skills into a host's directory.
- `internal/skills/evals/`: the eval harness behind `skills evals`.

## Where the history is

Each change since the first commit has a plan, a step log and a post-report under `../.ai/phase-N/` in the workspace. The post-report compares the change against the spec. Start there to learn why something is the way it is.
