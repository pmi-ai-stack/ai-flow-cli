## 13. Dependencies

- Stdlib first. Before `go get`: is it in `slices`, `maps`, `strings`, `os`, `net/http`, `encoding/json`, `text/template` already?
- Allowed direct dependencies (justified by the spec): `github.com/spf13/cobra` (+ `pflag`), `github.com/santhosh-tekuri/jsonschema/v6`, `gopkg.in/yaml.v3`, `golang.org/x/sync` (errgroup), `github.com/google/go-cmp` (tests), `github.com/rogpeppe/go-internal` (testscript). Adding anything else needs a sentence in the PR: why not stdlib, licence, maintenance, transitive size.
- **Tools are shelled out, never imported**: no `conftest`, `terraform`, `helm`, `kubectl`, `client-go`, `go-git` as libraries. The binary stays small and tool versions belong to the developer. (`git` too — through `Exec`.)
- Not allowed: `viper` (global state, case-folding surprises), `logrus`/`zap` (slog), `pkg/errors` (stdlib), `testify` (stdlib + go-cmp is the house style; do not mix), anything requiring cgo (`CGO_ENABLED=0` is the build).
- `go mod tidy` produces no diff in CI; `govulncheck` weekly and on PR; Renovate/Dependabot for the module.

