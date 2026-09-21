## 7. Determinism, files, the handoff directory

These rules exist because a skill compares bytes and a run must be resumable.

- **One encoder for every artifact**, in `internal/handoff` (`func WriteJSON(path string, v any) error`): `json.MarshalIndent` on a struct whose fields are declared in the order the schema lists them, two-space indent, `\n` at the end, `SetEscapeHTML(false)`. For dynamic maps (`counts{}`, `blast_radius`) `encoding/json` already sorts keys — but prefer structs. **[tool: `TestByteStable` per package]**
- **No timestamps, hostnames, PIDs, temp paths, absolute paths or map iteration order in any results file.** Time and paths go to `events.jsonl` (append-only, `O_APPEND`, one JSON object per line, never read by a stage). Paths inside artifacts are relative to the repo or the handoff dir.
- **Sort every slice before writing**: scanner findings by `(file, line, rule_id)`, standards findings by `(layer, check_id, file, line)`, changeset actions by `(addon, target, kind, name)`, `kubectl diff` output by `kind/namespace/name`. Use `slices.SortFunc` with a `cmp.Or`/`strings.Compare` chain; the comparator is a named function with its own test.
- **Atomic write**: `os.CreateTemp` in the *same* directory → write → `f.Sync()` → `f.Close()` (check the error) → `os.Rename`. Validate against the embedded schema *before* the rename so a malformed file never exists on disk. **[review]**
- **The state machine is code.** `handoff.Status.Transition(stage, to)` is the single function that mutates a stage state; it returns `ErrStageNotAllowed` for anything the diagram in the spec forbids. Transitions are a table, tested cell by cell. Skip rules (`should-run`) are a table in `internal/handoff` (or `internal/ops`), also tested per row.
- **Idempotent by construction**: writes overwrite identically; `ops policy scaffold` refuses when the target exists; `ticket update --marker` edits an existing comment; `stage reset` removes exactly the files the spec lists.
- `path/filepath` always (`filepath.Join`), never `+ "/"`. `os.Root` (1.24+) when working inside `render/` or a temp worktree, so `..` and symlinks cannot escape. Temp worktrees are removed with `defer` + `context.WithoutCancel`.
- `//go:embed` for the config template, schemas, `output.md` template and policy baselines. Embedded files are read once into package-level **immutable** values (`embed.FS` is fine; a parsed schema behind `sync.OnceValue` is fine).
- YAML: `gopkg.in/yaml.v3` for pack manifests and config-ish files; `sigs.k8s.io/yaml` only if a package needs `kubectl` semantics — never both in one package. Unknown fields are errors (`KnownFields(true)`, `DisallowUnknownFields()`), because a typo in `config.json` must fail `config validate`, not silently default.
- JSON into typed structs with tags; `map[string]any` only in the plan-JSON normalizer and similar "strip these keys" code, and it never leaves that function.

