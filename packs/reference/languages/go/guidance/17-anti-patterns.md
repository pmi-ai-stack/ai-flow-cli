## 17. Anti-patterns specific to this CLI

| You'll be tempted to… | Instead |
| --- | --- |
| `fmt.Println(result)` in a command | Return the value; `internal/envelope` prints it once |
| Print a progress line to stdout "just for humans" | stderr, via the logger; `--human` is the human mode |
| `exec.Command("terraform", "apply", …)` "only for the sandbox" | It does not exist. The allowlist test will fail; e2e sandboxes are the harness runner's job and still never `apply` |
| `exec.Command("sh", "-c", "helm template … \| kubeconform")` | Two `Exec.Run` calls, pipe the bytes in Go |
| `time.Now()` in a results struct "for debugging" | `events.jsonl` |
| `map[string]any` for the changeset because Terraform's plan JSON is big | Typed structs for what you read; the normalizer strips the rest and is the only `map[string]any` |
| Re-derive the branch name in a stage | `ticket.json` has it; `handoff` computes it once |
| Ask the developer with `bufio.NewReader(os.Stdin)` | Exit 4 with `needs_input`; the skill asks |
| Log the kubeconfig path *and* content at `Debug` "because verbose" | Path yes, content never; `redact` still runs |
| A `util` package for "shared helpers" between addons | Put the helper in `internal/ops` next to the type it serves, or duplicate five lines |
| An `Addon` interface method the spec does not have | Change the spec first; the skill directory contract mirrors it |
| Sorting in the writer only when tests fail | Sort at the boundary where the slice is produced, with a named comparator and its own test |
| `if err := os.WriteFile(path, b, 0o644)` for a results file | `handoff.WriteJSON` — atomic + schema-validated + deterministic |
| Editing `.golangci.yml` to make the `standards` stage pass on this repo | The guard hook blocks it for a reason; fix the finding or propose a waiver |

