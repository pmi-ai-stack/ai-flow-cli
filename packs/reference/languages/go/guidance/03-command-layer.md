## 3. The command layer (Cobra)

```go
// internal/cli/root.go
func NewRootCmd(stdout, stderr io.Writer, env Env) *cobra.Command {
    var g globalFlags
    cmd := &cobra.Command{
        Use:           "ai-flow",
        Short:         "Ticket-to-PR pipeline commands for Claude Code skills",
        SilenceUsage:  true,  // runtime errors never dump --help onto stdout
        SilenceErrors: true,  // main prints the envelope exactly once
        PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
            return g.setup(stderr) // logger, home, config path
        },
    }
    cmd.SetOut(stdout)
    cmd.SetErr(stderr)
    cmd.PersistentFlags().StringVar(&g.home, "home", defaultHome(env), "root holding config.json and one dir per ticket")
    cmd.PersistentFlags().BoolVar(&g.human, "human", false, "text output instead of the JSON envelope")
    cmd.PersistentFlags().BoolVar(&g.dryRun, "dry-run", false, "print intended tracker/git/PR side effects without executing")
    cmd.PersistentFlags().BoolVarP(&g.verbose, "verbose", "v", false, "provider and tool logs on stderr, secrets redacted")
    cmd.AddCommand(newTicketCmd(&g), newRepoCmd(&g), newGitCmd(&g), newStageCmd(&g), newOpsCmd(&g), newStandardsCmd(&g), /* … */)
    return cmd
}
```

- **`RunE`, never `Run`.** No `os.Exit`, `log.Fatal`, `panic` inside a command. **[review; `forbidigo` can flag `os.Exit`/`log.Fatal` outside `main`]**
- **Constructors, not `init()` + globals.** `newOpsDryRunCmd(g *globalFlags) *cobra.Command`. Tests build the tree, `cmd.SetArgs(...)`, `cmd.SetOut(&buf)`, `cmd.ExecuteContext(ctx)` and assert on the envelope in `buf`. **[tool: `gochecknoinits`]**
- **`cmd.Context()`** is the only context a command uses; `main` creates it with `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`.
- **The command function is thin:** parse flags → build options → call one function in `internal/<domain>` → write the result into the envelope. Business logic must be callable and testable without Cobra. **[review]**
- **Output discipline:** the envelope is written with `cmd.OutOrStdout()` by *one* function in `internal/envelope`; nothing else ever writes to stdout. `fmt.Print*` without an explicit writer is forbidden in the module. **[tool: `forbidigo` on `fmt.Print`, `fmt.Println`, `fmt.Printf`, `print`, `println`]**
- Flags are `--kebab-case`; short aliases only for `-v`; every flag has a help string in the same words the spec uses. Positional `KEY` validated with `cobra.ExactArgs(1)` and the key sanitizer from `internal/handoff`.
- `--human` prints text; `--json` is the default and is *not* a flag that can be turned off. `--dry-run` is honoured by tracker/git/PR side effects and ignored by read-only OPS commands (say so in the flag help).

### 3.1 Envelope and exit codes

```go
// internal/envelope
type Envelope struct {
    OK         bool        `json:"ok"`
    Command    string      `json:"command"`
    Result     any         `json:"result"`
    NeedsInput *NeedsInput `json:"needs_input"`
    Error      *Error      `json:"error"`
    Warnings   []string    `json:"warnings"`
}

type Error struct {
    Code    string   `json:"code"`    // "usage", "missing_artifact", "blast_radius_gate", …
    Message string   `json:"message"`
    Hint    string   `json:"hint,omitempty"`
    Details []string `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func (e *Error) ExitCode() int  { return exitFor(e.Code) } // table, tested
```

- Every failure that the spec assigns an `error.code` to is a `*envelope.Error` (or a typed error `envelope` knows how to map) created **where the condition is detected**, not reconstructed in `main` from a string. Unknown/unexpected errors map to `internal`, exit 1.
- The `error.code → exit code` mapping is **one table** in `internal/envelope` with a test that every code named in the spec is present and maps to exactly one of 1–6. Adding a code means adding a table row and a test row in the same PR.
- `needs_input` is exit 4 and always carries `question`, `options`, `free_text` and a `resume_with` that is a complete, copy-pasteable `ai-flow …` command. The CLI never asks a question itself. **[review]**
- `warnings` are for things the run continued through (expired waiver ignored, writable kube context); never for things that changed the result.
- `Envelope` is marshalled with the same deterministic encoder as every other artifact (§7). `Result` is always a concrete struct, never `map[string]any`.

