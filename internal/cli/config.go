package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

func newConfigCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage ~/.ai-flow/config.json",
	}
	cmd.AddCommand(newConfigInitCmd(cio))
	cmd.AddCommand(newConfigValidateCmd(cio))
	return cmd
}

type configInitResult struct {
	Path string `json:"path"`
}

func newConfigInitCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write the embedded config template to ~/.ai-flow/config.json",
		Args:  cobra.NoArgs,
		RunE: wrap("config init", cio.stdout, cio.exitCode, func(cmd *cobra.Command, _ []string) (Outcome, error) {
			f := FlagsFrom(cmd.Context())
			path, err := config.Init(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: configInitResult{Path: path}}, nil
		}),
	}
}

type configValidateResult struct {
	Path          string `json:"path"`
	SchemaVersion int    `json:"schema_version"`
}

func newConfigValidateCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate ~/.ai-flow/config.json against every documented rule",
		Args:  cobra.NoArgs,
		RunE: wrap("config validate", cio.stdout, cio.exitCode, func(cmd *cobra.Command, _ []string) (Outcome, error) {
			f := FlagsFrom(cmd.Context())
			path := config.ResolvePath(f.Home, f.Config)

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}

			warnings, err := config.Validate(cfg, config.DefaultKubeconfigPath())
			if err != nil {
				details := flattenJoinedErrors(err)
				return Outcome{}, &envelope.Error{
					Code:    "config_invalid",
					Message: fmt.Sprintf("%s: %d validation error(s)", path, len(details)),
					Details: details,
				}
			}

			return Outcome{
				Result:   configValidateResult{Path: path, SchemaVersion: cfg.SchemaVersion},
				Warnings: warnings,
			}, nil
		}),
	}
}

// multiError is the interface errors.Join's return value implements
// (Go 1.20+) — checked directly rather than through errors.As, which
// has no equivalent for "unwrap into many errors" the way it does for
// a single Unwrap() error chain.
type multiError interface {
	Unwrap() []error
}

// flattenJoinedErrors turns config.Validate's aggregated error into one
// message per violation, so every rule failure reaches
// envelope.Error.Details instead of being flattened into one long
// newline-joined string.
func flattenJoinedErrors(err error) []string {
	if m, ok := err.(multiError); ok {
		var out []string
		for _, e := range m.Unwrap() {
			out = append(out, flattenJoinedErrors(e)...)
		}
		return out
	}
	return []string{err.Error()}
}
