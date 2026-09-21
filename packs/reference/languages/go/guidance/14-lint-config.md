## 14. Lint configuration

`.golangci.yml` (v2). Strict on bugs, quiet on taste. Every linter here points at a rule above.

```yaml
version: "2"
run:
  timeout: 5m
  build-tags: [integration, mutation, ops]
linters:
  default: none
  enable:
    - errcheck          # §4.1
    - govet             # printf, copylocks, hostport, tests
    - staticcheck       # ST1005 error strings, deprecated APIs, nil derefs
    - ineffassign
    - unused
    - errorlint         # §4.4 errors.Is/As, %w
    - bodyclose         # tracker/host HTTP
    - nolintlint        # every //nolint has a reason
    - gochecknoinits    # §3, §9
    - gochecknoglobals  # §9 (allowlist + embedded assets excluded below)
    - forbidigo         # §3, §8 — see settings
    - gosec             # os/exec, file perms; tuned
    - misspell
    - thelper
    - testifylint       # keeps testify out, effectively
  settings:
    errcheck:
      exclude-functions:
        - (io.Closer).Close     # reads only; writes check Close explicitly
    nolintlint:
      require-explanation: true
      require-specific: true
    forbidigo:
      forbid:
        - pattern: '^fmt\.Print(|f|ln)$'
          msg: "stdout is the envelope; use cmd.OutOrStdout()/ErrOrStderr() or the logger"
        - pattern: '^(print|println)$'
        - pattern: '^os\.Exit$'
          msg: "only main exits"
        - pattern: '^log\.(Fatal|Fatalf|Fatalln|Panic|Panicf|Panicln)$'
        - pattern: '^exec\.Command(Context)?$'
          msg: "every subprocess goes through internal/ops/exec"
        - pattern: '^os\.Setenv$'
          msg: "build cmd.Env explicitly; t.Setenv in tests"
        - pattern: '^time\.Sleep$'
          msg: "use ctx deadlines / tickers"
        - pattern: '^time\.Now$'
          msg: "results files carry no wall-clock data; inject a clock or write to events.jsonl only"
    gosec:
      excludes: [G204]          # subprocess with variable args — by design, behind the allowlist
  exclusions:
    generated: lax
    rules:
      - path: ^cmd/ai-flow/main\.go$
        linters: [forbidigo, gochecknoglobals]  # os.Exit lives here; version/commit/date are -X injection targets
      - path: ^internal/ops/exec/
        text: 'exec\.Command'   # the one allowed place
      - path: ^internal/ops/exec/allowlist\.go$
        linters: [gochecknoglobals]
      - path: ^internal/ops/exec/denied\.go$
        linters: [gochecknoglobals]  # CommandHints is a lookup table, not mutable state
      - path: ^internal/envelope/exitcode\.go$
        linters: [gochecknoglobals]  # codeExitCode is a lookup table, not mutable state
      - path: ^internal/handoff/events\.go$
        text: 'time\.Now'       # events.jsonl is the one place wall-clock is allowed
      - path: _test\.go$
        linters: [errcheck, bodyclose, gochecknoglobals, forbidigo, gosec]
formatters:
  enable: [gofmt, goimports]
  settings:
    goimports:
      local-prefixes: [github.com/pmi-ai-stack/ai-flow-cli]
```

Suppressions are rare, specific and explained: `//nolint:gosec // G304: path comes from the validated handoff layout`.

