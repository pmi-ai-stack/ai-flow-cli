package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/gitops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation/gremlins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation/mutmut"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation/pitest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation/stryker"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/unit"
)

// mutationTools maps the tool name internal/detect.Mutation returns to
// its mutation.Tool implementation — the plan's design decision 1:
// internal/mutation itself never imports any of the four, so this
// package picks the concrete type by name, the same way ticket.go's
// buildTracker and pr.go's buildHost already pick a provider.
var mutationTools = map[string]mutation.Tool{
	"gremlins": gremlins.Tool{},
	"stryker":  stryker.Tool{},
	"mutmut":   mutmut.Tool{},
	"pitest":   pitest.Tool{},
}

func newMutationCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mutation",
		Short: "Measure test adequacy on changed code via mutation testing",
	}
	cmd.AddCommand(newMutationScopeCmd(cio))
	cmd.AddCommand(newMutationRunCmd(cio))
	cmd.AddCommand(newMutationSummarizeCmd(cio))
	return cmd
}

// mutationReportDTO is mutation-reports/round-<N>/<repo>.json's own
// shape — the plan's design decision 4 as refined during
// implementation: internal/mutation.Report normalized straight to
// JSON by `mutation run`, so `mutation summarize` never needs to
// re-detect a repository's tool or re-parse a tool-native report
// format a second time. Unlike mutation-results.json this file has no
// embedded schema (like infra-test-reports/*, the Handoff directory
// table names no schema for either) — handoff.WriteRaw, not
// WriteArtifact.
type mutationReportDTO struct {
	Tool      string                `json:"tool"`
	Total     int                   `json:"total"`
	Killed    int                   `json:"killed"`
	Survivors []mutationSurvivorDTO `json:"survivors"`
}

type mutationSurvivorDTO struct {
	ID   string `json:"id"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// emptyIfNil turns a nil slice into an empty one — every artifact
// this file writes uses "[]", never "null", for an array field with
// nothing in it.
func emptyIfNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func toMutationReportDTO(tool string, r mutation.Report) mutationReportDTO {
	dto := mutationReportDTO{Tool: tool, Total: r.Total, Killed: r.Killed}
	for _, s := range r.Survivors {
		dto.Survivors = append(dto.Survivors, mutationSurvivorDTO{ID: s.ID, File: s.File, Line: s.Line})
	}
	dto.Survivors = emptyIfNil(dto.Survivors)
	return dto
}

// mutationResultsEquivalent mirrors mutation-results.schema.json's
// equivalent[] item exactly.
type mutationResultsEquivalent struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// mutationResultsAccepted mirrors mutation-results.schema.json's
// accepted_below_threshold object exactly.
type mutationResultsAccepted struct {
	By     string `json:"by"`
	Reason string `json:"reason"`
}

// mutationResultsRepo mirrors mutation-results.schema.json's
// repositories[] item exactly.
type mutationResultsRepo struct {
	Repo       string                      `json:"repo"`
	Tool       string                      `json:"tool"`
	Score      float64                     `json:"score"`
	Threshold  int                         `json:"threshold"`
	Survivors  int                         `json:"survivors"`
	Killed     int                         `json:"killed"`
	Equivalent []mutationResultsEquivalent `json:"equivalent"`
}

// mutationResultsArtifact mirrors mutation-results.schema.json exactly.
type mutationResultsArtifact struct {
	Round                  int                      `json:"round"`
	Repositories           []mutationResultsRepo    `json:"repositories"`
	AcceptedBelowThreshold *mutationResultsAccepted `json:"accepted_below_threshold"`
}

// mutationTarget is one repository mutation run/scope found a tool
// for, with a non-empty changed-code scope.
type mutationTarget struct {
	repo  repoArtifactRepository
	tool  string
	scope mutation.Scope
	diff  gitops.DiffResult
}

// mutationScopeForRepo computes r's detected mutation tool (if any)
// and its diff-scoped mutable files — the one place detection and
// scoping happen, shared by mutationTargetsFor (which then filters
// out an undetected tool or an empty scope) and `mutation scope`
// (which reports every repository as-is, detected or not). detected
// is false exactly when detect.Mutation found no tool; err is
// returned unchanged from whichever of detect.Mutation/gitops.Diff
// failed, before ok is even meaningful.
func mutationScopeForRepo(ctx context.Context, g *gitops.GitOps, cfg *config.Config, r repoArtifactRepository) (tool string, detected bool, scope mutation.Scope, diff gitops.DiffResult, err error) {
	dr := toDetectRepo(r)
	m, ok, err := detect.Mutation(dr)
	if err != nil || !ok {
		return "", ok, mutation.Scope{}, gitops.DiffResult{}, err
	}
	diff, err = g.Diff(ctx, r.LocalPath, cfg.Host.DefaultBaseBranch)
	if err != nil {
		return "", false, mutation.Scope{}, gitops.DiffResult{}, err
	}
	return m.Tool, true, mutation.ScopeFor(dr, m.Tool, diff.Files), diff, nil
}

// mutationTargetsFor computes, for every repository in artifact, its
// detected tool and diff-scoped mutable files — the plan's design
// decision 3: the same ScopeFor call `mutation scope` (read-only) and
// `mutation run` both use, so the two can never disagree. Only
// repositories with a detected tool AND at least one mutable changed
// file are returned; the rest are silently absent (no --repo flag
// exists on any of the three mutation commands — see the plan's §1.2).
func mutationTargetsFor(ctx context.Context, g *gitops.GitOps, cfg *config.Config, artifact *repoArtifact) ([]mutationTarget, error) {
	var targets []mutationTarget
	for _, r := range artifact.Repositories {
		tool, ok, scope, diff, err := mutationScopeForRepo(ctx, g, cfg, r)
		if err != nil {
			return nil, err
		}
		if !ok || len(scope.Files) == 0 {
			continue
		}
		targets = append(targets, mutationTarget{repo: r, tool: tool, scope: scope, diff: diff})
	}
	return targets, nil
}

type mutationScopeRepoResult struct {
	Repo     string   `json:"repo"`
	Detected bool     `json:"detected"`
	Tool     string   `json:"tool,omitempty"`
	Files    []string `json:"files,omitempty"`
}

type mutationScopeResult struct {
	Key          string                    `json:"key"`
	Repositories []mutationScopeRepoResult `json:"repositories"`
}

// newMutationScopeCmd is read-only (the plan's design decision 3): it
// prints exactly what `mutation run` would compute internally and
// writes nothing — mirroring `stage should-run`'s own
// diagnostic-command shape.
func newMutationScopeCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "scope KEY",
		Short: "Print each in-scope repository's detected mutation tool and changed-code scope",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("mutation scope", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			g := buildGitOps(cfg)
			result := mutationScopeResult{Key: key}
			for _, r := range artifact.Repositories {
				tool, ok, scope, _, err := mutationScopeForRepo(cmd.Context(), g, cfg, r)
				if err != nil {
					return Outcome{}, err
				}
				if !ok {
					result.Repositories = append(result.Repositories, mutationScopeRepoResult{Repo: r.Name, Detected: false})
					continue
				}
				result.Repositories = append(result.Repositories, mutationScopeRepoResult{Repo: r.Name, Detected: true, Tool: tool, Files: emptyIfNil(scope.Files)})
			}
			return Outcome{Result: result}, nil
		}),
	}
}

// sha256Hex is the "tree hash" the plan's design decision 7 defines:
// a hash over a repository's own diff text against base, narrowly
// scoped to the one precondition that needs it (mutation run deciding
// whether to re-run a repository's unit suite before mutation
// testing) — not a general-purpose concept this codebase reuses
// elsewhere yet.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ensureMutationTargetsTestsGreen is the plan's design decision 7:
// before executing any mutation tool, every target's test-results.json
// entry must be green and current. A target whose recorded tree_hash
// no longer matches its current diff (or has no entry at all) has its
// unit suite re-run via the same internal/verify/unit.Run phase 6
// built; every other repository's existing entry in test-results.json
// is preserved untouched on a partial refresh — test.go's own
// `test run --repo R` overwrites the whole file with only the
// repositories it touched, which would be wrong here since mutation
// run acts on every in-scope repository in one call.
func ensureMutationTargetsTestsGreen(ctx context.Context, cfg *config.Config, ticketDir string, artifact *repoArtifact, targets []mutationTarget, x exec.Execer) error {
	existingData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "test-results.json"))
	if err != nil {
		return err
	}
	var existing testResultsArtifact
	if existingData != nil {
		if err := json.Unmarshal(existingData, &existing); err != nil {
			return fmt.Errorf("cli: decode test-results.json: %w", err)
		}
	}
	byRepo := map[string]testResultsRepo{}
	for _, r := range existing.Repositories {
		byRepo[r.Repo] = r
	}

	changed := false
	for _, tgt := range targets {
		hash := sha256Hex(tgt.diff.Diff)
		cur, ok := byRepo[tgt.repo.Name]
		if ok && cur.TreeHash == hash {
			if cur.Failed > 0 {
				return &envelope.Error{Code: "missing_artifact", Message: "test-results.json is not green for " + tgt.repo.Name + "; fix failing tests before running mutation testing"}
			}
			continue
		}

		dr := toDetectRepo(tgt.repo)
		m, ok2, err := detect.Runners(dr)
		if err != nil {
			return err
		}
		if !ok2 {
			return &envelope.Error{Code: "missing_artifact", Message: "no unit test runner detected for " + tgt.repo.Name + "; cannot verify tests are green before mutation testing"}
		}
		res, err := unit.Run(ctx, dr, m, x, cfg.Defaults.TestTimeout())
		if err != nil {
			return err
		}
		entry := toTestResultsRepo(res)
		entry.TreeHash = hash
		byRepo[tgt.repo.Name] = entry
		changed = true
		if res.Failed > 0 {
			return &envelope.Error{Code: "missing_artifact", Message: "test-results.json is not green for " + tgt.repo.Name + "; fix failing tests before running mutation testing"}
		}
	}

	if !changed {
		return nil
	}
	merged := testResultsArtifact{}
	for _, r := range artifact.Repositories {
		if e, ok := byRepo[r.Name]; ok {
			merged.Repositories = append(merged.Repositories, e)
		}
	}
	merged.Repositories = emptyIfNil(merged.Repositories)
	_, err = handoff.WriteArtifact(ticketDir, "test-results.json", "test-results", merged)
	return err
}

// latestMutationRound reports the highest round-<N> directory under
// ticketDir/mutation-reports/, or 0 when none exists yet.
func latestMutationRound(ticketDir string) (int, error) {
	base := filepath.Join(ticketDir, "mutation-reports")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("mutation: read %s: %w", base, err)
	}
	num := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var n int
		if _, serr := fmt.Sscanf(e.Name(), "round-%d", &n); serr == nil && n > num {
			num = n
		}
	}
	return num, nil
}

func mutationRoundDir(ticketDir string, round int) string {
	return filepath.Join(ticketDir, "mutation-reports", "round-"+strconv.Itoa(round))
}

type mutationRunResult struct {
	Key          string   `json:"key"`
	Round        int      `json:"round"`
	Repositories []string `json:"repositories"`
}

func newMutationRunCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "run KEY",
		Short: "Run the detected mutation tool over each target's changed-code scope, and write mutation-reports/",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("mutation run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			g := buildGitOps(cfg)
			targets, err := mutationTargetsFor(cmd.Context(), g, cfg, artifact)
			if err != nil {
				return Outcome{}, err
			}
			if len(targets) == 0 {
				return Outcome{}, &envelope.Error{Code: "tool_missing", Message: "no in-scope repository has both a detected mutation tool and changed mutable-language files"}
			}

			if f.Simulate {
				var names []string
				for _, tgt := range targets {
					names = append(names, tgt.repo.Name+":"+tgt.tool)
				}
				return Outcome{Result: map[string]any{"key": key, "targets": names}}, nil
			}

			x := newExecer()
			if err := ensureMutationTargetsTestsGreen(cmd.Context(), cfg, ticketDir, artifact, targets, x); err != nil {
				return Outcome{}, err
			}

			prevRound, err := latestMutationRound(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			round := prevRound + 1
			roundDir := mutationRoundDir(ticketDir, round)

			var written []string
			for _, tgt := range targets {
				tool := mutationTools[tgt.tool]
				report, err := tool.Run(cmd.Context(), toDetectRepo(tgt.repo), tgt.scope, x, cfg.Defaults.MutationTimeout())
				if err != nil {
					return Outcome{}, err
				}
				dto := toMutationReportDTO(tgt.tool, report)
				if _, err := handoff.WriteRaw(roundDir, tgt.repo.Name+".json", dto); err != nil {
					return Outcome{}, err
				}
				written = append(written, tgt.repo.Name)
			}
			sort.Strings(written)

			return Outcome{Result: mutationRunResult{Key: key, Round: round, Repositories: written}}, nil
		}),
	}
}

func roundScore(f float64) float64 {
	return math.Round(f*10) / 10
}

// parsedMutationReport is one repository's round-<N> report, already
// read back from mutation-reports/.
type parsedMutationReport struct {
	repo   string
	report mutationReportDTO
}

// loadRoundReports reads every *.json file under roundDir — one per
// repository mutation run wrote a report for.
func loadRoundReports(roundDir string) ([]parsedMutationReport, error) {
	entries, err := os.ReadDir(roundDir)
	if err != nil {
		return nil, fmt.Errorf("mutation: read %s: %w", roundDir, err)
	}
	var parsed []parsedMutationReport
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := handoff.ReadArtifact(filepath.Join(roundDir, e.Name()))
		if err != nil {
			return nil, err
		}
		var dto mutationReportDTO
		if err := json.Unmarshal(data, &dto); err != nil {
			return nil, fmt.Errorf("mutation: decode %s: %w", e.Name(), err)
		}
		parsed = append(parsed, parsedMutationReport{repo: strings.TrimSuffix(e.Name(), ".json"), report: dto})
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].repo < parsed[j].repo })
	return parsed, nil
}

func newMutationSummarizeCmd(cio cmdIO) *cobra.Command {
	var equivalentID, reasonFlag string
	var acceptFlag bool
	cmd := &cobra.Command{
		Use:   "summarize KEY",
		Short: "Parse the latest mutation-reports/ round into mutation-results.json, recording equivalents or accepting a below-threshold score",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("mutation summarize", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			if equivalentID != "" && acceptFlag {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--equivalent and --accept are mutually exclusive"}
			}
			if equivalentID != "" && reasonFlag == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--equivalent requires --reason"}
			}
			if acceptFlag && reasonFlag == "" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: "--accept requires --reason"}
			}

			f := FlagsFrom(cmd.Context())
			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			artifact, err := loadRepoArtifact(ticketDir)
			if err != nil {
				return Outcome{}, err
			}

			round, err := latestMutationRound(ticketDir)
			if err != nil {
				return Outcome{}, err
			}
			if round == 0 {
				return Outcome{}, &envelope.Error{Code: "missing_artifact", Message: "no mutation-reports/ round found; run `ai-flow mutation run " + key + "` first"}
			}
			parsed, err := loadRoundReports(mutationRoundDir(ticketDir, round))
			if err != nil {
				return Outcome{}, err
			}

			existingData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "mutation-results.json"))
			if err != nil {
				return Outcome{}, err
			}
			var existing mutationResultsArtifact
			sameRound := false
			if existingData != nil {
				if err := json.Unmarshal(existingData, &existing); err != nil {
					return Outcome{}, fmt.Errorf("cli: decode mutation-results.json: %w", err)
				}
				sameRound = existing.Round == round
			}
			equivByRepo := map[string][]mutationResultsEquivalent{}
			var carriedAccepted *mutationResultsAccepted
			if sameRound {
				for _, rr := range existing.Repositories {
					equivByRepo[rr.Repo] = rr.Equivalent
				}
				carriedAccepted = existing.AcceptedBelowThreshold
			}

			if equivalentID != "" {
				matchRepo := ""
				matches := 0
				for _, p := range parsed {
					for _, s := range p.report.Survivors {
						if s.ID == equivalentID {
							matches++
							matchRepo = p.repo
						}
					}
				}
				switch {
				case matches == 0:
					return Outcome{}, &envelope.Error{Code: "usage", Message: "no surviving mutant with id " + equivalentID + " in round " + strconv.Itoa(round)}
				case matches > 1:
					return Outcome{}, &envelope.Error{Code: "usage", Message: "mutant id " + equivalentID + " is ambiguous across repositories in round " + strconv.Itoa(round)}
				}
				for _, e := range equivByRepo[matchRepo] {
					if e.ID == equivalentID {
						return Outcome{}, &envelope.Error{Code: "usage", Message: equivalentID + " is already recorded as equivalent"}
					}
				}
				equivByRepo[matchRepo] = append(equivByRepo[matchRepo], mutationResultsEquivalent{ID: equivalentID, Reason: reasonFlag})
			}

			accepted := carriedAccepted
			if acceptFlag {
				accepted = &mutationResultsAccepted{By: currentUser(), Reason: reasonFlag}
			}

			result := mutationResultsArtifact{Round: round, AcceptedBelowThreshold: accepted}
			belowThreshold := false
			for _, p := range parsed {
				threshold := config.ResolveMutationThreshold(cfg, artifact.Team, p.repo)
				equiv := equivByRepo[p.repo]
				denom := p.report.Total - len(equiv)
				score := 100.0
				if denom > 0 {
					score = roundScore(float64(p.report.Killed) / float64(denom) * 100)
				}
				if score < float64(threshold) {
					belowThreshold = true
				}
				result.Repositories = append(result.Repositories, mutationResultsRepo{
					Repo: p.repo, Tool: p.report.Tool, Score: score, Threshold: threshold,
					Survivors: len(p.report.Survivors), Killed: p.report.Killed,
					Equivalent: emptyIfNil(equiv),
				})
			}
			result.Repositories = emptyIfNil(result.Repositories)

			if _, err := handoff.WriteArtifact(ticketDir, "mutation-results.json", "mutation-results", result); err != nil {
				return Outcome{}, err
			}

			if belowThreshold && accepted == nil {
				options := []string{"add-tests", "accept"}
				if round >= cfg.Defaults.MutationMaxRounds {
					options = []string{"accept"}
				}
				err := &envelope.Error{Code: "mutation_below_threshold", Message: "mutation score is below threshold for one or more repositories", Details: options}
				return Outcome{NeedsInput: needsInputFor(err, key)}, err
			}

			return Outcome{Result: result}, nil
		}),
	}
	cmd.Flags().StringVar(&equivalentID, "equivalent", "", "mark a surviving mutant id as equivalent (requires --reason)")
	cmd.Flags().StringVar(&reasonFlag, "reason", "", "reason for --equivalent or --accept")
	cmd.Flags().BoolVar(&acceptFlag, "accept", false, "accept the current mutation score below threshold (requires --reason)")
	return cmd
}
