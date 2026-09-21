## 21. Config loading (`internal/config`)

`internal/config` is not built yet (see `ai-flow-cli/AGENTS.md` repository status); these are the rules to apply when it lands, not a description of existing code. Once it exists, treat its own code and tests as authoritative over this section, matching the precedent `AGENTS.md` already sets for `internal/envelope`, `internal/cli` and `internal/ops/exec`.

- **Precedence is fixed and explicit**: flags > env > config file > embedded defaults. The merge happens in one function (`config.Load`), not scattered across command `RunE`s.
- **Validate once, at load time**, and fail with a single aggregated error (`errors.Join`, see `04-errors.md` §8) rather than failing on the first bad field — a skill running `config validate` wants every problem in one `needs_input`/error response, not one round trip per field.
- **No interactive prompts, ever** — consistent with the top-level `ai-flow-cli/AGENTS.md` invariant that the CLI never asks a question; an invalid or incomplete config is exit 4 with `needs_input`, not a stdin read.
- Secrets (tracker tokens, host tokens) are read from env only, never from the config file or a flag — flags show up in `ps`, and the config file is not a secret store. This matches `06-logging.md`'s redaction rule: a secret-shaped field is validated for *presence*, never logged or echoed back by `--print-config`/`doctor`.
- `--print-config`/`config validate`'s human output redacts every field whose name matches `token`, `password`, `secret`, `key` — same redaction boundary as logging, not a separate implementation.
- Config values that are durations are parsed once, at load time, into `time.Duration` (`ops.tool_timeout_minutes` → a `time.Duration` field) — call sites never re-parse a string or a literal.
