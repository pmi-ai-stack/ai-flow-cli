package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// explainListResult is `explain` without a code: every code's entry.
type explainListResult struct {
	Codes []envelope.Explanation `json:"codes"`
}

func newExplainCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "explain [CODE]",
		Short: "Say what an error.code means and how to recover; without a code, list them all",
		Args:  cobra.MaximumNArgs(1),
		RunE: wrap("explain", cio.stdout, cio.exitCode, func(_ *cobra.Command, args []string) (Outcome, error) {
			if len(args) == 0 {
				return Outcome{Result: explainListResult{Codes: envelope.Explanations()}}, nil
			}
			e, ok := envelope.Explain(args[0])
			if !ok {
				return Outcome{}, &envelope.Error{
					Code:    "usage",
					Message: fmt.Sprintf("%q is not an error.code; run `ai-flow explain` to list them", args[0]),
				}
			}
			return Outcome{Result: e}, nil
		}),
	}
}
