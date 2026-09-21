package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
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

// Handler is the signature every ai-flow subcommand implements. A
// returned *envelope.Error carries a documented error.code straight
// through to the envelope; any other non-nil error is an unexpected
// internal failure and is mapped to error.code "internal".
type Handler func(cmd *cobra.Command, args []string) (Outcome, error)

// NewRoot builds the root command: global flags, PersistentPreRunE
// plumbing them into the context, and every subcommand. main prints
// nothing itself — every RunE prints exactly one envelope via wrap, and
// *exitCode is set to the code main() must exit with.
func NewRoot(bi BuildInfo, stdout, stderr io.Writer, exitCode *int) *cobra.Command {
	var (
		homeFlag     string
		configFlag   string
		simulateFlag bool
		verboseFlag  bool
		humanFlag    bool
	)

	root := &cobra.Command{
		Use:           "ai-flow",
		Short:         "ai-flow drives the deterministic side of the app and OPS pipelines",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.PersistentFlags().StringVar(&homeFlag, "home", defaultHome(), "ai-flow home directory")
	root.PersistentFlags().StringVar(&configFlag, "config", "", "path to config.json (default <home>/config.json)")
	root.PersistentFlags().BoolVar(&simulateFlag, "simulate", false, "print side effects without performing them, then exit 0")
	root.PersistentFlags().BoolVar(&verboseFlag, "verbose", false, "tool logs to stderr, secrets redacted")
	root.PersistentFlags().BoolVar(&humanFlag, "human", false, "human-readable output instead of --json (the default)")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		f := Flags{
			Home:     homeFlag,
			Config:   configFlag,
			Simulate: simulateFlag,
			Verbose:  verboseFlag,
			Human:    humanFlag,
		}
		cmd.SetContext(WithFlags(cmd.Context(), f))
		return nil
	}

	root.AddCommand(newVersionCmd(bi, stdout, exitCode))

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
		}

		if f.Human {
			return envelope.PrintHuman(stdout, env)
		}
		return envelope.Print(stdout, env)
	}
}
