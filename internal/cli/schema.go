package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	schemapkg "github.com/pmi-ai-stack/ai-flow-cli/internal/schema"
)

func newSchemaCmd(stdout io.Writer, exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "schema <artifact>",
		Short: "Print the embedded JSON Schema for an artifact",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("schema", stdout, exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			artifact := args[0]
			v, err := schemapkg.Print(artifact)
			if err != nil {
				if errors.Is(err, schemapkg.ErrUnknownArtifact) {
					return Outcome{}, &envelope.Error{
						Code:    "usage",
						Message: fmt.Sprintf("unknown artifact %q", artifact),
						Details: []string{"one of: " + strings.Join(schemapkg.Artifacts(), ", ")},
					}
				}
				return Outcome{}, err
			}
			return Outcome{Result: v}, nil
		}),
	}
}
