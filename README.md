# ai-flow CLI

The deterministic side of the ai-flow pipeline. One Go binary; every tracker, git, git-host, infra and test-tool side effect goes through it. The agent (Claude Code / Cursor skills in the sibling [`ai-flow`](../ai-flow) plugin repo) carries judgement only and talks to this binary exclusively through `ai-flow <noun> <verb> --json`.

## Install

```sh
curl -fsSL -o install.sh https://raw.githubusercontent.com/pmi-ai-stack/ai-flow-cli/3785e5fca8fa7c463d568da0cde3c57483526d82/install.sh
sh install.sh
```

The URL is pinned to a commit, not a branch, so what you run is a fixed, reviewable script; the same SHA is `INSTALL_SH_SHA` in the plugin's `hooks/cli-version.sh` and both move together whenever `install.sh` changes. With [`cosign`](https://docs.sigstore.dev/cosign/system_config/installation/) on PATH, `install.sh` also verifies `checksums.txt`'s keyless signature against this repository's release workflow at the installed tag; `-s` makes that check mandatory. Each release also carries an SPDX SBOM per archive.

Or download a release archive from the [releases page](https://github.com/pmi-ai-stack/ai-flow-cli/releases) and verify it against `checksums.txt` yourself. `install.sh -v vX.Y.Z -d DIR` pins a specific release and install directory — this is what the [`ai-flow` plugin's `session-start.sh`](../ai-flow/hooks/session-start.sh) calls automatically, resolved against its own manifest's `cli_version` range, so most people never run this by hand.

### Homebrew (macOS, Linux)

```sh
brew install --cask pmi-ai-stack/tap/ai-flow
```

The cask is published to `pmi-ai-stack/homebrew-tap` by each release. The binary is not notarized by Apple, so the cask removes macOS's quarantine attribute after install. For provenance, verify the archive against the release's signed `checksums.txt`, as `install.sh` does.

### Container image

```sh
docker run --rm -v "$PWD:/src" -w /src ghcr.io/pmi-ai-stack/ai-flow-cli:vX.Y.Z standards check .
```

The image is `linux/amd64` and `linux/arm64`. It holds the same static binary on a distroless base, runs as a non-root user, and has `/usr/local/bin/ai-flow` as its entrypoint. Each manifest is signed keylessly by the release workflow, the same identity as `checksums.txt`, and carries an SBOM attestation:

```sh
cosign verify ghcr.io/pmi-ai-stack/ai-flow-cli:vX.Y.Z \
  --certificate-identity https://github.com/pmi-ai-stack/ai-flow-cli/.github/workflows/release.yml@refs/tags/vX.Y.Z \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

It carries **no check tools**. Run as is, every check whose tool is missing is reported as an open finding. For a real CI image, copy the binary into an image that already pins your tools:

```dockerfile
COPY --from=ghcr.io/pmi-ai-stack/ai-flow-cli:vX.Y.Z /usr/local/bin/ai-flow /usr/local/bin/ai-flow
```

### Windows

Each release builds `ai-flow_<version>_windows_amd64.zip` and `…_windows_arm64.zip`. There is no installer: `install.sh` stops on Windows and points here. In PowerShell:

```powershell
$v = "X.Y.Z"; $arch = "amd64"   # or arm64
$base = "https://github.com/pmi-ai-stack/ai-flow-cli/releases/download/v$v"
Invoke-WebRequest "$base/ai-flow_${v}_windows_$arch.zip" -OutFile ai-flow.zip
Invoke-WebRequest "$base/checksums.txt" -OutFile checksums.txt
$want = (Select-String "ai-flow_${v}_windows_$arch.zip" checksums.txt).Line.Split(" ")[0]
if ((Get-FileHash ai-flow.zip -Algorithm SHA256).Hash.ToLower() -ne $want) { throw "checksum mismatch" }
Expand-Archive ai-flow.zip -DestinationPath "$env:LOCALAPPDATA\ai-flow"
```

Then add `%LOCALAPPDATA%\ai-flow` to `PATH`. With cosign installed, verify `checksums.txt` first, using `checksums.txt.sig`, `checksums.txt.pem` and the same `cosign verify-blob` identity as `install.sh`.

**What works on Windows today:**
- The CLI itself, e.g. `standards check <dir>` in CI.

**What does not:**
- The plugin's hook scripts need a POSIX shell (Git Bash or WSL). Without one the guard fails open, by design.
- OPS tooling on Windows is unverified.

## Configure

The ticket pipeline reads `~/.ai-flow/config.json`. `config init` writes a template for your tracker, and `config validate` checks it:

```sh
ai-flow config init                    # Jira: AI_FLOW_JIRA_EMAIL and AI_FLOW_JIRA_TOKEN
ai-flow config init --tracker github   # GitHub Issues: AI_FLOW_GITHUB_TOKEN
ai-flow config validate
```

For GitHub Issues, replace the placeholder in `tracker.repositories`. It maps a ticket-key prefix to the repository whose issue numbers it names: `{ "APP": "acme/app" }` makes `APP-42` issue #42 of `acme/app`. The status classes are labels (`in progress`, `in review`), and an open issue with neither label counts as `open`. `ai-flow doctor` checks the token and that the tracker is reachable.

## Use in CI

Check a repository against the standards pack from its own pipeline — no ticket, no `config.json` and no `~/.ai-flow` needed:

```sh
ai-flow standards check .
```

It runs every check that applies to the languages and infrastructure it detects (terraform, Kubernetes manifests, Helm, kustomize, GitHub Actions, Jenkins) and exits 6 naming every open `must` finding. The check tools themselves (`gitleaks`, `conftest`, `checkov`, your linters) must be on `PATH`; one that is missing is reported as an open finding at that check's severity, never skipped silently. Checks that need a rendering (`input: render` — most Kubernetes and pipeline policy) cannot run without a ticket and are listed under `result.skipped`. `--pack DIR` or `--pack <git-url>@<ref>` checks against your own pack instead of the built-in reference one; `--require-instructions` fails when the repository has no `AGENTS.md`/`CLAUDE.md`.

`--sarif FILE` also writes the findings as SARIF 2.1.0, whether the run passes or fails:
- open findings are results at their severity (`must` → error, `should` → warning, `may` → note);
- waived ones are marked as suppressed;
- an empty run lets code scanning close fixed alerts.

Stdout is the same envelope either way.

### GitHub Actions

```yaml
jobs:
  standards:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write   # only for the SARIF upload
    steps:
      - uses: actions/checkout@<sha> # v4
      # Install the check tools your pack runs, e.g. gitleaks and conftest.
      - id: ai-flow
        uses: pmi-ai-stack/ai-flow-cli@vX.Y.Z
      - if: always()
        uses: github/codeql-action/upload-sarif@<sha> # v4
        with:
          sarif_file: ${{ steps.ai-flow.outputs.sarif }}
```

What the action does:
- It installs the release named by its own tag, using the `install.sh` at that tag, with the cosign signature check mandatory. It then runs `standards check`.
- It writes a verdict to the job summary and fails the step while a `must` finding is open.

Inputs:
- `path` (default `.`)
- `pack`
- `require-instructions`
- `sarif` (default `ai-flow.sarif`)
- `version`, to install a release other than the action's tag

Outputs: `exit-code` and `sarif`.

Linux and macOS runners only.

### Jenkins

With the [Warnings Next Generation](https://plugins.jenkins.io/warnings-ng/) plugin and cosign on the agent:

```groovy
pipeline {
  agent any
  stages {
    stage('Standards') {
      steps {
        sh '''
          curl -fsSL -o install.sh https://raw.githubusercontent.com/pmi-ai-stack/ai-flow-cli/vX.Y.Z/install.sh
          sh install.sh -s -v vX.Y.Z -d "$WORKSPACE/.bin"
          "$WORKSPACE/.bin/ai-flow" standards check . --sarif ai-flow.sarif
        '''
      }
    }
  }
  post {
    always {
      recordIssues(tools: [sarif(pattern: 'ai-flow.sarif')])
    }
  }
}
```

The `sh` step fails the stage while a `must` finding is open (exit 6). `recordIssues` publishes every open finding either way.

## The contract

- Every command prints exactly one JSON envelope on stdout (`{ok, command, result, needs_input, error, warnings}`); progress goes to stderr. `--json` is the default. `--human` is for a person at a terminal: `doctor`, `standards check` and `status` print readable text, every other command prints its result as indented JSON under a header, and failures always print the code, message and any question.
- Exit codes 0–6 carry a documented `error.code`; exit 4 always means `needs_input` — the CLI never asks a question itself.
- Nothing is committed, pushed, transitioned or applied unless `status.json` records the gate that authorizes it.

See [`AGENTS.md`](AGENTS.md) for the packages that implement this contract and how this repo is organized.

## Build from source

```sh
go build -o dist/ai-flow ./cmd/ai-flow
```

Requires Go 1.27+. See `packs/reference/languages/go/guidance/` for the coding rules and `AGENTS.md` for the pre-PR command.
