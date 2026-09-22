package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills/install"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills/lint"
)

func newSkillsCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Lint and install ai-flow skills",
	}
	cmd.AddCommand(newSkillsLintCmd(cio))
	cmd.AddCommand(newSkillsInstallCmd(cio))
	return cmd
}

type skillsLintFinding struct {
	Rule    string `json:"rule"`
	Skill   string `json:"skill,omitempty"`
	File    string `json:"file,omitempty"`
	Message string `json:"message"`
}

type skillsLintResult struct {
	Dir      string              `json:"dir"`
	Skills   []string            `json:"skills"`
	Findings []skillsLintFinding `json:"findings"`
}

// newSkillsLintCmd builds `skills lint <dir>`. dir/skills/*/SKILL.md is
// checked against every rule in ai-flow/docs/skill-authoring.md's own
// table — see internal/skills/lint's package doc comment for how. A
// hard finding is exit 6 invalid_result, per the plan's design
// decision 8: "Fix the file; show error.details[]" is a verbatim
// description of what a lint failure needs. Soft body-length notices
// (internal/skills/lint.Result.Warnings) are envelope warnings and
// never fail the command.
func newSkillsLintCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "lint <dir>",
		Short: "Lint every skill under dir/skills/ against the authoring standard",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("skills lint", cio.stdout, cio.exitCode, func(_ *cobra.Command, args []string) (Outcome, error) {
			dir := args[0]
			res, err := lint.Lint(dir)
			if err != nil {
				return Outcome{}, err
			}

			if len(res.Findings) > 0 {
				details := make([]string, len(res.Findings))
				for i, f := range res.Findings {
					details[i] = formatFinding(f)
				}
				sort.Strings(details)
				return Outcome{}, &envelope.Error{
					Code:    "invalid_result",
					Message: fmt.Sprintf("%s: %d lint violation(s) across %d skill(s)", dir, len(res.Findings), len(res.Skills)),
					Details: details,
				}
			}

			// res.Findings is empty here — the branch above already
			// returned on any hard finding.
			return Outcome{
				Result:   skillsLintResult{Dir: dir, Skills: emptyIfNil(res.Skills), Findings: []skillsLintFinding{}},
				Warnings: res.Warnings,
			}, nil
		}),
	}
}

func formatFinding(f lint.Finding) string {
	switch {
	case f.Skill == "" && f.File == "":
		return fmt.Sprintf("%s: %s", f.Rule, f.Message)
	case f.File == "":
		return fmt.Sprintf("%s: %s: %s", f.Rule, f.Skill, f.Message)
	default:
		return fmt.Sprintf("%s: %s/%s: %s", f.Rule, f.Skill, f.File, f.Message)
	}
}

type skillsInstallResult struct {
	From   string   `json:"from"`
	Into   string   `json:"into"`
	Skills []string `json:"skills"`
}

// newSkillsInstallCmd builds `skills install --from <dir> --into <dir>`
// — the no-plugin install path the Hosts table names. --from is a root
// directory holding a skills/ subdirectory (the same meaning `skills
// lint <dir>` gives its own argument — the plugin repository itself, or
// "." run from inside it), not the skills/ directory directly:
// `ai-flow skills install --from ai-flow --into .claude/skills` serves
// both hosts, `--into .agents/skills` is the Cursor-only form.
func newSkillsInstallCmd(cio cmdIO) *cobra.Command {
	var fromFlag, intoFlag string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Copy every skill from one directory into a host's skills directory",
		Args:  cobra.NoArgs,
		RunE: wrap("skills install", cio.stdout, cio.exitCode, func(_ *cobra.Command, _ []string) (Outcome, error) {
			if fromFlag == "" || intoFlag == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--from and --into are both required"}
			}
			res, err := install.Install(fromFlag, intoFlag)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: skillsInstallResult{From: res.From, Into: res.Into, Skills: emptyIfNil(res.Skills)}}, nil
		}),
	}
	cmd.Flags().StringVar(&fromFlag, "from", "", "source root holding skills/<name>/SKILL.md (the plugin repository, not its skills/ subdirectory)")
	cmd.Flags().StringVar(&intoFlag, "into", "", "destination skills directory, e.g. .claude/skills")
	return cmd
}
