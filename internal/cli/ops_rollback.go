package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/rollback"
)

// opsRollbackResult is this command's own envelope shape — the
// artifact plus the ticket key.
type opsRollbackResult struct {
	Key string `json:"key"`
	rollback.Result
}

func newOpsRollbackCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback KEY",
		Short: "Derive the revert path from the already-computed changeset: git revert per repository, tool-level rollback per target, non-revertible addresses and the watch list",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops rollback", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			if f.Simulate {
				return Outcome{Result: opsRollbackResult{Key: key, Result: emptyRollbackResult()}}, nil
			}

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			repoArt, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			tcs, _, err := loadTargetChangesets(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			reg := buildOpsRegistry(f, cfg)
			repos := make([]rollback.Repository, len(repoArt.Repositories))
			for i, r := range repoArt.Repositories {
				repos[i] = rollback.Repository{Name: r.Name}
			}
			targets := make([]rollback.TargetInput, len(tcs))
			for i, tc := range tcs {
				addon, _ := reg.Lookup(tc.Target.Addon)
				targets[i] = rollback.TargetInput{Target: tc.Target, Env: envFor(tc.Target), Changeset: tc.Changeset, Addon: addon}
			}

			result, err := rollback.Derive(cmd.Context(), repos, targets)
			if err != nil {
				return Outcome{}, err
			}

			if _, err := handoff.WriteArtifact(ticketDir, "rollback-results.json", "rollback-results", result); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: opsRollbackResult{Key: key, Result: result}}, nil
		}),
	}
}

func emptyRollbackResult() rollback.Result {
	return rollback.Result{
		Repositories:  []rollback.RepositoryRevert{},
		Targets:       []rollback.TargetResult{},
		NonRevertible: []string{},
		WatchList:     []string{},
	}
}

// nonRevertibleHeading is the exact heading rollback-plan.md's own
// "Non-revertible" section (Workflow table: "Trigger criteria, Steps,
// Non-revertible, Watch list, Owner") must carry for plan_check to
// find it.
const nonRevertibleHeading = "## Non-revertible"

// checkRollbackPlanCheck implements plan_check: every non_revertible
// address rollback-results.json records must appear as a mitigation
// somewhere under rollback-plan.md's own "## Non-revertible" heading.
// A no-op when there is nothing to check (empty non_revertible) — a
// missing rollback-plan.md in that case is not this function's
// concern. rollbackResultsData is the caller's already-read
// rollback-results.json bytes (checkPrimaryArtifact's own return
// value), so this never re-reads the file.
func checkRollbackPlanCheck(ticketDir string, rollbackResultsData []byte) error {
	var result rollback.Result
	if err := json.Unmarshal(rollbackResultsData, &result); err != nil {
		return fmt.Errorf("cli: decode rollback-results.json: %w", err)
	}
	if len(result.NonRevertible) == 0 {
		return nil
	}

	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "rollback-plan.md"))
	if err != nil {
		return err
	}
	section := nonRevertibleSection(string(data))

	var missing []string
	for _, addr := range result.NonRevertible {
		if !strings.Contains(section, addr) {
			missing = append(missing, addr)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return &envelope.Error{
		Code:    "invalid_result",
		Message: "rollback-plan.md is missing a mitigation under \"## Non-revertible\" for one or more addresses",
		Details: missing,
	}
}

// nonRevertibleSection extracts the body text between
// nonRevertibleHeading and the next "## " heading (or EOF) — a plain
// substring scan, the same style internal/standards/check/pr.go's own
// required-headings rule already uses, not a Markdown parser. Returns
// "" when the heading is absent at all (rollback-plan.md missing
// entirely, or present without this section) — every address then
// counts as missing.
func nonRevertibleSection(md string) string {
	idx := strings.Index(md, nonRevertibleHeading)
	if idx < 0 {
		return ""
	}
	rest := md[idx+len(nonRevertibleHeading):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	return rest
}
