package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

// ticketOnlyKinds are the three check kinds whose input is a ticket
// artifact (plan.md, commit-message.md, pr-body.md) rather than a
// repository's own tree — run once per ticket, never per repository
// (the phase-14 plan's design decision 4).
var ticketOnlyKinds = map[string]bool{"docs": true, "commit": true, "pr": true}

// addonScopedKinds are the two check kinds that run once per OPS
// target rather than once per repository — every kind-"policy" or
// kind-"scan" example in the spec's own pack format narrows by
// applies_to.addons, never applies_to.languages (the phase-25 plan's
// design decision 1).
var addonScopedKinds = map[string]bool{"policy": true, "scan": true}

// checksResultsWire is checks-results.json's own shape — check.Finding
// already mirrors the schema field for field (runner.go's own doc
// comment), so this is the one place internal/cli wraps it for
// handoff.WriteArtifact, exactly the way internal/cli/mutation.go
// builds mutation-results.json's own wire struct rather than asking
// internal/mutation to own a handoff-writing method (the phase-14
// plan's design decision under §3.4 — internal/standards/check cannot
// import internal/standards back, so the writer lives here).
type checksResultsWire struct {
	Findings []check.Finding `json:"findings"`
}

func newStandardsCheckCmd(cio cmdIO) *cobra.Command {
	var fix bool
	var stageFlag string
	cmd := &cobra.Command{
		Use:   "check KEY_OR_DIR",
		Short: "Run every applicable standards check and write checks-results.json (or, for a bare directory, print findings for CI)",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("standards check", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			arg := args[0]
			f := FlagsFrom(cmd.Context())

			stage := stageFlag
			if stage != "checks" && stage != "ship" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: fmt.Sprintf("--stage %q must be \"checks\" or \"ship\"", stage)}
			}

			if key, ok := ticketKeyIfExists(f.Home, arg); ok {
				return runStandardsCheckTicket(cmd.Context(), f, key, stage, fix)
			}
			return runStandardsCheckDir(cmd.Context(), f, arg, fix)
		}),
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "apply format-kind autofixes at the source")
	cmd.Flags().StringVar(&stageFlag, "stage", "checks", "which stage's findings this run replaces: checks or ship")
	return cmd
}

// ticketKeyIfExists reports whether arg is a ticket key with an
// existing handoff directory under home — the dispatch between the
// ticket form and the directory form. A key-shaped string with no
// handoff directory yet (before `ticket fetch` has ever run) falls
// through to the directory form and fails there as "no such
// directory," a clear enough error for what is, in practice, always a
// developer mistake rather than a real ambiguity: every real call to
// `standards check KEY` happens well after the ticket directory exists
// (locate must have already run for there to be a repo.json to
// resolve against at all).
func ticketKeyIfExists(home, arg string) (string, bool) {
	if err := handoff.ValidateKey(arg); err != nil {
		return "", false
	}
	info, err := os.Stat(handoff.TicketDir(home, arg))
	if err != nil || !info.IsDir() {
		return "", false
	}
	return arg, true
}

type standardsCheckResult struct {
	Key      string          `json:"key,omitempty"`
	Dir      string          `json:"dir,omitempty"`
	Stage    string          `json:"stage"`
	Fixed    int             `json:"fixed"`
	Open     int             `json:"open"`
	Waived   int             `json:"waived"`
	Findings []check.Finding `json:"findings"`
}

func toStandardsCheckResult(key, dir, stage string, findings []check.Finding) standardsCheckResult {
	res := standardsCheckResult{Key: key, Dir: dir, Stage: stage, Findings: emptyIfNil(findings)}
	for _, f := range findings {
		switch f.Status {
		case "fixed":
			res.Fixed++
		case "waived":
			res.Waived++
		case "open":
			res.Open++
		}
	}
	return res
}

// runStandardsCheckTicket runs every effective, applicable check for
// key's resolution, merges the result into checks-results.json with
// per-stage replacement, and writes it. It always exits 0 — blocking
// on an open must happens at `stage complete checks`/`stage approve
// ship`, which already read this file independently (the phase-14
// plan's design decision 8).
func runStandardsCheckTicket(ctx context.Context, f Flags, key, stage string, fix bool) (Outcome, error) {
	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return Outcome{}, err
	}

	e, packs, err := resolveForTicket(f, key, cfg)
	defer standards.ClosePacks(packs)
	if err != nil {
		return Outcome{}, err
	}

	ticketDir := handoff.TicketDir(f.Home, key)
	artifact, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return Outcome{}, err
	}

	fresh, err := runEffectiveChecks(ctx, e, artifact.Repositories, stage, fix, ticketDir, newExecer(), cfg.Ops.ToolTimeout())
	if err != nil {
		return Outcome{}, err
	}

	existing, err := readChecksResults(ticketDir)
	if err != nil {
		return Outcome{}, err
	}
	merged := mergeFindings(existing, fresh, stage)

	if _, err := handoff.WriteArtifact(ticketDir, "checks-results.json", "checks-results", checksResultsWire{Findings: merged}); err != nil {
		return Outcome{}, err
	}

	return Outcome{Result: toStandardsCheckResult(key, "", stage, fresh)}, nil
}

// runStandardsCheckDir is the ticket-free CI form: org+repo layers
// only (no team — there is no repo.json to name one), languages
// detected from dir itself, always stage "checks", no handoff write.
// Exits 6 invalid_result naming every open must finding — the
// phase-14 plan's design decision 9, settling the plan's own "Decides:
// standards check in the repositories' own CI" as yes, enforced here.
func runStandardsCheckDir(ctx context.Context, f Flags, dir string, fix bool) (Outcome, error) {
	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return Outcome{}, err
	}

	name := repoNameFromDir(dir)
	repo := standards.RepoRef{Name: name, LocalPath: dir}

	languages, err := detect.Languages(detect.Repo{Name: name, LocalPath: dir})
	if err != nil {
		return Outcome{}, fmt.Errorf("cli: detect languages in %s: %w", dir, err)
	}
	waivers, err := standards.LoadWaivers(name, dir)
	if err != nil {
		return Outcome{}, err
	}

	e, packs, err := standards.Resolve(standards.ResolveInput{
		Home: f.Home, Cfg: cfg, Repos: []standards.RepoRef{repo},
		Languages: languages, Waivers: waivers,
	})
	defer standards.ClosePacks(packs)
	if err != nil {
		return Outcome{}, err
	}

	repos := []repoArtifactRepository{{Name: name, LocalPath: dir}}
	findings, err := runEffectiveChecks(ctx, e, repos, "checks", fix, "", newExecer(), cfg.Ops.ToolTimeout())
	if err != nil {
		return Outcome{}, err
	}

	if open := openMustFindings(findings); len(open) > 0 {
		return Outcome{}, &envelope.Error{
			Code:    "invalid_result",
			Message: fmt.Sprintf("%s: %d open must finding(s)", dir, len(open)),
			Details: formatFindingDetails(open),
		}
	}

	return Outcome{Result: toStandardsCheckResult("", dir, "checks", findings)}, nil
}

func repoNameFromDir(dir string) string {
	return filepath.Base(filepath.Clean(dir))
}

// runEffectiveChecks runs every effective, non-shadowed check that
// applies to stage, against every repo it applies to. A waived check
// still runs — its findings are recorded with status "waived" rather
// than never appearing at all, so checks-results.json (and the
// output.md/PR body render built on it) shows what was found and
// accepted, not just that a waiver exists. secrets is
// standards.UnwaivableKind, so this path never actually rewrites its
// status in practice.
func runEffectiveChecks(ctx context.Context, e *standards.Effective, repos []repoArtifactRepository, stage string, fix bool, ticketDir string, x exec.Execer, timeout time.Duration) ([]check.Finding, error) {
	// Detected once per repository, not once per (check, repository)
	// pair: a repository's own languages never change during one run,
	// but the loop below considers every applicable check against
	// every repository, and re-running detect.Languages' handful of
	// os.Stat/os.ReadDir calls per check would multiply that work by
	// the number of applicable non-ticket-only checks for no reason.
	// One map keyed by repo name, not two: languages and local path are
	// both per-repository facts the per-repo and addon-scoped loops
	// below both need, so one lookup serves both instead of a second,
	// parallel map carrying nothing detect.Languages didn't already
	// require visiting this repository for.
	repoInfo := make(map[string]repoCheckInfo, len(repos))
	for _, r := range repos {
		langs, err := detect.Languages(toDetectRepo(r))
		if err != nil {
			return nil, err
		}
		repoInfo[r.Name] = repoCheckInfo{Langs: langs, LocalPath: r.LocalPath}
	}

	// opsTargets/renderHashes feed the addon-scoped ("policy"/"scan")
	// dispatch branch below — both tolerant of absence (ticketDir ==
	// "" for the directory/CI form, or an ordinary ticket with no OPS
	// content at all), in which case no addon-scoped check ever finds
	// a target to run against, correctly.
	var opsTargets []ops.Target
	var renderHashes map[string]string
	if ticketDir != "" {
		targetsArt, err := loadOpsTargetsTolerant(ticketDir)
		if err != nil {
			return nil, err
		}
		if targetsArt != nil {
			// changedOrOneTarget with no --target flag is exactly "every
			// target ops-targets.json marks changed" — the same scope
			// `ops drift`/`ops dry-run` already use (internal/cli/ops.go).
			opsTargets, err = changedOrOneTarget(targetsArt.Targets, "")
			if err != nil {
				return nil, err
			}
		}
		hashes, err := loadRenderDriftResultsTolerant(ticketDir)
		if err != nil {
			return nil, err
		}
		renderHashes = make(map[string]string, len(hashes))
		for key, t := range hashes {
			renderHashes[key] = t.Hash
		}
	}

	var out []check.Finding

	for _, c := range e.Checks {
		if !c.Effective() {
			continue
		}
		if !standards.Applies(c.Check.AppliesTo, e.Languages, e.Addons, stage) {
			continue
		}
		runner, ok := check.ForKind(c.Kind)
		if !ok {
			continue // no runner for this kind
		}

		if ticketOnlyKinds[c.Kind] {
			findings, err := runner.Run(ctx, check.RunInput{Check: c, Stage: stage, Fix: fix, TicketDir: ticketDir}, x, timeout)
			if err != nil {
				return nil, err
			}
			out = append(out, decorateFindings(findings, c, stage)...)
			continue
		}

		if addonScopedKinds[c.Kind] {
			findings, err := runAddonScopedCheck(ctx, c, opsTargets, repoInfo, ticketDir, renderHashes, stage, fix, runner, x, timeout)
			if err != nil {
				return nil, err
			}
			out = append(out, decorateFindings(findings, c, stage)...)
			continue
		}

		for _, r := range repos {
			if !standards.AppliesToAnyStage(c.Check.AppliesTo, repoInfo[r.Name].Langs, e.Addons) {
				continue
			}
			findings, err := runner.Run(ctx, check.RunInput{
				Check: c, Repo: check.Repo{Name: r.Name, LocalPath: r.LocalPath},
				Stage: stage, Fix: fix, TicketDir: ticketDir,
			}, x, timeout)
			if err != nil {
				return nil, err
			}
			out = append(out, decorateFindings(findings, c, stage)...)
		}
	}
	return out, nil
}

// repoCheckInfo is the per-repository data runEffectiveChecks's own
// dispatch (both the per-repository loop and runAddonScopedCheck)
// needs, keyed by repository name — one map instead of two, since both
// fields come from the same one-per-repository pass over repos.
type repoCheckInfo struct {
	Langs     []string
	LocalPath string
}

// runAddonScopedCheck runs c — a "policy" or "scan" kind check, always
// scoped to one OPS target rather than one repository — against every
// changed target (the same scope render-drift/dry-run already use)
// whose addon c's own applies_to selects. Check.Input == "render" needs
// a rendering render-drift already wrote (render.RedactedDir over the
// hash render-drift-results.json recorded for that target); a target
// with no such hash yet (its addon has no Render/Validate capability,
// or render-drift has not run this run) is silently skipped — "nothing
// to check here yet," not a failure.
func runAddonScopedCheck(
	ctx context.Context, c standards.EffectiveCheck, targets []ops.Target, repoInfo map[string]repoCheckInfo,
	ticketDir string, renderHashes map[string]string, stage string, fix bool,
	runner check.CheckRunner, x exec.Execer, timeout time.Duration,
) ([]check.Finding, error) {
	var out []check.Finding
	for _, t := range targets {
		info, ok := repoInfo[t.Repo]
		if !ok {
			continue // repo.json names no such repository — nothing to check
		}
		if !standards.AppliesToAnyStage(c.Check.AppliesTo, info.Langs, []string{t.Addon}) {
			continue
		}

		// check.Target, not check.Repo: runPolicy/runScan (the only
		// runners addonScopedKinds ever dispatches to) read Target.Repo/
		// Target.SourceDir exclusively, never RunInput.Repo — the same
		// carried-repository-name that ticketOnlyKinds' own dispatch
		// leaves as the zero value two branches above.
		target := check.Target{
			Key: ops.TargetKey(t), Addon: t.Addon, Repo: t.Repo,
			SourceDir: filepath.Join(info.LocalPath, t.Path),
		}
		if c.Input == "render" {
			hash := renderHashes[target.Key]
			if hash == "" {
				continue // not rendered this run — nothing to check
			}
			target.RenderDir = render.RedactedDir(ticketDir, t, hash)
		}

		findings, err := runner.Run(ctx, check.RunInput{
			Check: c, Target: target, Stage: stage, Fix: fix, TicketDir: ticketDir,
		}, x, timeout)
		if err != nil {
			return nil, err
		}
		out = append(out, findings...)
	}
	return out, nil
}

// decorateFindings fills in what only the resolution knows — a runner
// sees one check and nothing about its place in the layered
// resolution, so CheckID/Layer/Kind/Stage are set here, not by the
// runner. Severity defaults to the check's own declared severity only
// when the runner left it empty: every phase-14 runner does (format,
// lint, ... never set a per-finding severity of their own), but the
// scan runners (phase 25) compute a real, CapSeverity-capped severity
// per finding — "capped at the check entry's severity," not "always
// exactly the check entry's severity," the spec's own wording for
// scan. A waived check's otherwise-open findings become status
// "waived" (see runEffectiveChecks's own doc comment).
func decorateFindings(findings []check.Finding, c standards.EffectiveCheck, stage string) []check.Finding {
	for i := range findings {
		findings[i].CheckID = c.ID
		findings[i].Layer = c.Layer
		findings[i].Kind = c.Kind
		if findings[i].Severity == "" {
			findings[i].Severity = c.Severity
		}
		findings[i].Stage = stage
		if findings[i].Tool == "" {
			findings[i].Tool = c.Tool
		}
		if c.Waived && findings[i].Status == "open" {
			findings[i].Status = "waived"
		}
	}
	return findings
}

// readChecksResults reads ticketDir's existing checks-results.json, or
// nil when there is none yet.
func readChecksResults(ticketDir string) ([]check.Finding, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "checks-results.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var wire checksResultsWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("cli: decode checks-results.json: %w", err)
	}
	return wire.Findings, nil
}

// mergeFindings keeps every existing finding whose Stage differs from
// stage (a --stage ship run must not erase a --stage checks run, and
// vice versa — the spec's own per-stage replacement rule) and replaces
// the rest with fresh, sorted by (layer, check_id, file, line) per the
// spec's own byte-stability rule.
func mergeFindings(existing, fresh []check.Finding, stage string) []check.Finding {
	merged := make([]check.Finding, 0, len(existing)+len(fresh))
	for _, f := range existing {
		if f.Stage != stage {
			merged = append(merged, f)
		}
	}
	merged = append(merged, fresh...)
	sort.SliceStable(merged, func(i, j int) bool {
		a, b := merged[i], merged[j]
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		if a.CheckID != b.CheckID {
			return a.CheckID < b.CheckID
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	return emptyIfNil(merged)
}

func openMustFindings(findings []check.Finding) []check.Finding {
	var out []check.Finding
	for _, f := range findings {
		if f.Severity == "must" && f.Status == "open" {
			out = append(out, f)
		}
	}
	return out
}

func formatFindingDetails(findings []check.Finding) []string {
	details := make([]string, 0, len(findings))
	for _, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		if loc == "" {
			loc = f.Repo
		}
		details = append(details, fmt.Sprintf("%s: %s: %s", f.CheckID, loc, f.Message))
	}
	sort.Strings(details)
	return details
}
