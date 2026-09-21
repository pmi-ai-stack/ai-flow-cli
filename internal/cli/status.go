package cli

import (
	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// statusResult is `ai-flow status KEY`'s result: status.json's own
// content plus the computed next_stage/next_skill/next_command/
// next_state the orchestrator resumes from.
type statusResult struct {
	Key         string               `json:"key"`
	Stages      []handoff.StageEntry `json:"stages"`
	NextStage   *string              `json:"next_stage"`
	NextSkill   *string              `json:"next_skill"`
	NextCommand *string              `json:"next_command"`
	NextState   *string              `json:"next_state"`
	HandoffPath string               `json:"handoff_path"`
}

func newStatusCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "status KEY",
		Short: "Print a ticket's stage state and what runs next",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("status", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			st, err := handoff.LoadStatus(f.Home, key)
			if err != nil {
				return Outcome{}, err
			}

			result := statusResult{
				Key:         st.Key,
				Stages:      st.Stages,
				HandoffPath: handoff.TicketDir(f.Home, key),
			}

			nextStage, nextState := handoff.Next(st)
			if nextStage != nil {
				stageName := string(*nextStage)
				stateName := string(*nextState)
				result.NextStage = &stageName
				result.NextState = &stateName
				if def, ok := handoff.StageTableEntry(*nextStage); ok {
					result.NextSkill = def.Skill
					result.NextCommand = def.Command
				}
			}

			return Outcome{Result: result}, nil
		}),
	}
}
