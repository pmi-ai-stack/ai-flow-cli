package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/runlog"
)

// BuildInfo carries version metadata injected at link time via -X, with
// debug.ReadBuildInfo() as the runtime fallback (see version.go).
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// SchemaVersion is the artifact schema version this binary writes and
// validates against. It moves only when an artifact's shape changes.
const SchemaVersion = 1

// Outcome is what a Handler returns before it becomes an envelope.
type Outcome struct {
	Result     any
	NeedsInput *envelope.NeedsInput
	Warnings   []string
}

// cmdIO groups the two build-time values every leaf command constructor
// needs to hand to wrap: where the envelope goes, and where the exit
// code is recorded. Bundling them saves every newXCmd from repeating the
// same two-parameter pair — a shape the command catalog's remaining
// nouns (tracker, host, stage, ops, standards, ...) will otherwise
// repeat many more times over.
type cmdIO struct {
	stdout   io.Writer
	exitCode *int
}

// Handler is the signature every ai-flow subcommand implements. A
// returned *envelope.Error carries a documented error.code straight
// through to the envelope; any other non-nil error is an unexpected
// internal failure and is mapped to error.code "internal".
type Handler func(cmd *cobra.Command, args []string) (Outcome, error)

// NewRoot builds the root command: global flags, PersistentPreRunE
// plumbing them into the context, and every subcommand. main prints
// nothing itself — every RunE prints exactly one envelope via wrap, and
// *exitCode is set to the code main() must exit with. stdin is read by
// exactly two commands (hook guard, hook session-start — the two a
// host's own hook protocol pipes a JSON payload into); every other
// command ignores it.
func NewRoot(bi BuildInfo, stdin io.Reader, stdout, stderr io.Writer, exitCode *int) *cobra.Command {
	var (
		homeFlag     string
		configFlag   string
		simulateFlag bool
		verboseFlag  bool
		logFormat    string
		humanFlag    bool
		jsonFlag     bool
	)

	root := &cobra.Command{
		Use:           "ai-flow",
		Short:         "ai-flow drives the deterministic side of the app and OPS pipelines",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.PersistentFlags().StringVar(&homeFlag, "home", defaultHome(), "ai-flow home directory")
	root.PersistentFlags().StringVar(&configFlag, "config", "", "path to config.json (default <home>/config.json)")
	root.PersistentFlags().BoolVar(&simulateFlag, "simulate", false, "print side effects without performing them, then exit 0")
	root.PersistentFlags().BoolVar(&verboseFlag, "verbose", false, "tool logs to stderr, secrets redacted")
	root.PersistentFlags().StringVar(&logFormat, "log-format", "text", "format of the --verbose log on stderr: text, or json (one object per line with a run id)")
	root.PersistentFlags().BoolVar(&humanFlag, "human", false, "human-readable output instead of --json (the default)")
	// --json is the spec's own default output mode (Global flags), so on
	// its own it changes nothing — but it must be accepted: every skill
	// calls `ai-flow <noun> <verb> --json`, and session-start.sh parses
	// `ai-flow version --json`. Given alongside --human, --json wins, so
	// a caller that asks for the machine envelope always gets it.
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "JSON envelope output (the default; overrides --human)")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		f := Flags{
			Home:     homeFlag,
			Config:   configFlag,
			Simulate: simulateFlag,
			Verbose:  verboseFlag,
			Human:    humanFlag && !jsonFlag,
		}
		format, err := runlog.ParseFormat(logFormat)
		if err != nil {
			return err
		}
		ctx := WithFlags(cmd.Context(), f)
		if f.Verbose {
			command := strings.TrimPrefix(cmd.CommandPath(), root.Name()+" ")
			ctx = runlog.WithLogger(ctx, runlog.New(cmd.ErrOrStderr(), format, runlog.NewRunID(), command, redact.New(os.Environ()).String, handoff.Now))
		}
		cmd.SetContext(ctx)
		return nil
	}

	cio := cmdIO{stdout: stdout, exitCode: exitCode}
	root.AddCommand(newVersionCmd(bi, cio))
	root.AddCommand(newSchemaCmd(cio))
	root.AddCommand(newConfigCmd(cio))
	root.AddCommand(newStatusCmd(cio))
	root.AddCommand(newStageCmd(cio))
	root.AddCommand(newTicketCmd(cio))
	root.AddCommand(newRepoCmd(cio))
	root.AddCommand(newGitCmd(cio))
	root.AddCommand(newPRCmd(cio))
	root.AddCommand(newDetectCmd(cio))
	root.AddCommand(newTestCmd(cio))
	root.AddCommand(newE2ECmd(cio))
	root.AddCommand(newVisualCmd(cio))
	root.AddCommand(newMutationCmd(cio))
	root.AddCommand(newSummaryCmd(cio))
	root.AddCommand(newDoctorCmd(cio))
	root.AddCommand(newExplainCmd(cio))
	root.AddCommand(newHookCmd(bi, cio))
	root.AddCommand(newHooksCmd(cio))
	root.AddCommand(newSkillsCmd(cio))
	root.AddCommand(newStandardsCmd(cio))
	root.AddCommand(newOpsCmd(cio))

	return root
}

func defaultHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ai-flow"
	}
	return filepath.Join(home, ".ai-flow")
}

// wrap turns a Handler into a cobra RunE: it calls h, builds the
// envelope, prints it in the requested mode, and records the exit code
// the outcome maps to. stdout is the envelope, and nothing but the
// envelope — RunE never calls fmt.Print* itself.
func wrap(name string, stdout io.Writer, exitCode *int, h Handler) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		f := FlagsFrom(cmd.Context())

		outcome, err := h(cmd, args)

		var env envelope.Envelope
		if err != nil {
			var envErr *envelope.Error
			if !errors.As(err, &envErr) {
				envErr = &envelope.Error{Code: "internal", Message: err.Error()}
			}
			env = envelope.Fail(name, envErr)
			// A handler that returns a non-nil error may also populate
			// Outcome.NeedsInput — phase 5's first consumer is
			// routing_ambiguous, dirty_worktree and base_diverged,
			// which are exit 4 *and* carry a question/options/
			// resume_with on the same envelope (envelope.PrintHuman
			// already expects this — see its own !e.OK branch).
			env.NeedsInput = outcome.NeedsInput
			// Warnings ride along on a failure too: `standards check
			// <dir>` failing on its findings should still say it ran on
			// built-in defaults.
			if outcome.Warnings != nil {
				env.Warnings = outcome.Warnings
			}
			ec, ok := envelope.ExitCodeFor(envErr.Code)
			if !ok {
				ec = envelope.ExitInternal
			}
			*exitCode = int(ec)
		} else {
			env = envelope.Ok(name, outcome.Result)
			env.NeedsInput = outcome.NeedsInput
			if outcome.Warnings != nil {
				env.Warnings = outcome.Warnings
			}
			*exitCode = int(envelope.ExitOK)

			// events.jsonl's "tool runs" category (Handoff directory's
			// own words) — see the phase 11 plan's design decision 1.
			// Catalog()'s own SkipToolRunEvent field, not a name
			// convention re-derived here, decides which commands already
			// have a richer event of their own. Best-effort: a failure
			// here never turns a successful command into a failing
			// envelope, since this event is additive telemetry, not a
			// documented side effect in the exit-code contract.
			if !f.Simulate && len(args) > 0 && handoff.IsTicketScoped(name) && !handoff.SkipToolRunEvent(name) {
				_ = handoff.AppendEvent(handoff.TicketEventsPath(f.Home, args[0]), handoff.ToolRunEvent(name))
			}
		}

		// One funnel for every byte a command prints: rendered into a
		// buffer, redacted, then written. Provider error bodies
		// (statusError) and echoed configuration otherwise reached
		// stdout verbatim — a token in config.json came back out of
		// `config validate`.
		var buf bytes.Buffer
		render := envelope.Print
		if f.Human {
			render = envelope.PrintHuman
		}
		printErr := render(&buf, env)
		if _, err := stdout.Write(envelopeRedactor().Output(buf.Bytes())); err != nil {
			return err
		}
		return printErr
	}
}
