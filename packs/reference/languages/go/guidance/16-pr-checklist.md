## 16. PR checklist (paste into the PR template)

- [ ] `gofmt`, `go vet`, `golangci-lint`, `go test -race` green; `go mod tidy` no diff
- [ ] Every error returned with new context (`%w`) **or** handled — never both, never dropped
- [ ] `ctx` first on every I/O function; timeout from config on every external call
- [ ] No `os.Exit`, `log.Fatal`, `panic`, `init()`, mutable global, `fmt.Print*`, `exec.Command`, `time.Now` outside their one allowed file
- [ ] Every new `error.code` is in the envelope table + test; every `needs_input` has a runnable `resume_with`
- [ ] Every new artifact field is in the schema; struct tags match; byte-stability test updated
- [ ] New tool invocation: allowlist row + `TestAllowlistNeverMutatesPersistentState` green + recorded fixture for its parser
- [ ] New addon / check runner: registry line, fixture per parser, and the matching skill directory named in the PR
- [ ] Tests offline, table-driven, `t.TempDir`/`t.Setenv`, fakes not mocks; slow tiers behind build tags
- [ ] Secrets: read from env only, redacted in stderr/errors/artifacts; `doctor` prints presence only
- [ ] stdout = envelope only; stderr = logs; verified by the testscript for the command
- [ ] New dependency justified in the PR description; still `CGO_ENABLED=0`
- [ ] Doc comments on exported identifiers; `--help` text and this file updated if a rule changed; `cli_version` bumped if the contract changed
- [ ] Naming: no stutter, no `Get`, initialisms uppercase, booleans read as predicates (`18-naming.md`)
- [ ] Any new type parameter has a concrete caller in the same PR, and isn't standing in for an interface (`19-generics.md`)
- [ ] Config changes: precedence (flags > env > file > defaults) unchanged, validated once with an aggregated error, no secret in the file or a flag (`21-config.md`)

