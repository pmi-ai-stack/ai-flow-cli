package cli

import (
	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/render"
)

func newSummaryCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Render a ticket's summary",
	}
	cmd.AddCommand(newSummaryRenderCmd(cio))
	return cmd
}

// summaryRenderResult is `summary render`'s result shape.
type summaryRenderResult struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

func newSummaryRenderCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "render KEY",
		Short: "Render output.md and complete the output stage",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("summary render", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			ticketDir := handoff.TicketDir(f.Home, key)

			data, err := render.Load(ticketDir, key)
			if err != nil {
				return Outcome{}, err
			}

			path, err := handoff.WriteFile(ticketDir, "output.md", []byte(render.Output(data)))
			if err != nil {
				return Outcome{}, err
			}

			if err := driveCLIOnlyStage(f, key, handoff.StageOutput, "summary render"); err != nil {
				return Outcome{}, err
			}

			return Outcome{Result: summaryRenderResult{Key: key, Path: path}}, nil
		}),
	}
}
