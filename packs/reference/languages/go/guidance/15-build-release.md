## 15. Build and release

```sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(git describe --tags --always)" -o dist/ai-flow ./cmd/ai-flow
```

- `CGO_ENABLED=0`, `-trimpath`, `-s -w`: static, reproducible binaries, one per platform. A dependency that needs cgo is a dependency to drop.
- **Six platforms:** `linux`, `darwin`, `windows` × `amd64`, `arm64` — not four; Windows is in scope from phase 1's `.goreleaser.yaml` even though Windows support for the OPS tooling itself is verified later (P8/phase 30).
- `ai-flow version` prints `version`, `commit`, `date` from `-X` with `debug.ReadBuildInfo()` as the fallback, as an envelope like every other command.
- GoReleaser v2 on tag `vX.Y.Z`; `checksums.txt`; `install.sh` verifies the checksum before installing. The plugin's `cli_version` range in `plugin.json` is bumped in the same PR whenever the envelope, a schema, an exit code or the `Addon`/`Exec` contract changes — that is the definition of a breaking change for this binary.
- Schema files under `schema/` are versioned with the `schema_version` field the spec names — **1** as of phase 1 and phase 2, which is when the schemas first land; a schema shape change is a version bump plus an upgrade path in `config init`.
