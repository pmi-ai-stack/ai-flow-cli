package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
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
	var fix, requireInstructions bool
	var stageFlag, packFlag string
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
				if packFlag != "" || requireInstructions {
					return Outcome{}, &envelope.Error{Code: "usage", Message: "--pack and --require-instructions apply to the directory form only; a ticket resolves its own packs"}
				}
				return runStandardsCheckTicket(cmd.Context(), f, key, stage, fix)
			}
			if stage != "checks" {
				return Outcome{}, &envelope.Error{Code: "usage", Message: fmt.Sprintf("--stage %s applies to a ticket key only; a directory is checked at stage \"checks\"", stage)}
			}
			return runStandardsCheckDir(cmd.Context(), f, arg, dirCheckOptions{
				Fix: fix, Pack: packFlag, RequireInstructions: requireInstructions,
			})
		}),
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "apply format-kind autofixes at the source")
	cmd.Flags().StringVar(&stageFlag, "stage", "checks", "ticket form: which stage's findings this run replaces, checks or ship (the directory form is always checks)")
	cmd.Flags().StringVar(&packFlag, "pack", "", "directory form: use this pack as the org layer — a local directory, or <git-url>@<ref>")
	cmd.Flags().BoolVar(&requireInstructions, "require-instructions", false, "directory form: fail with missing_repo_instructions when the directory has no AGENTS.md or CLAUDE.md")
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
	// Skipped is the directory form's own list of checks that apply but
	// could not run there — today, every `input: render` check, since
	// the directory form renders nothing. Omitted when empty, and never
	// set by the ticket form, whose envelope is unchanged.
	Skipped []skippedCheck `json:"skipped,omitempty"`
}

// skippedCheck is one applicable check that did not run against one
// target, and why.
type skippedCheck struct {
	CheckID string `json:"check_id"`
	Target  string `json:"target"`
	Reason  string `json:"reason"`
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

	targets, err := ticketCheckTargets(ticketDir)
	if err != nil {
		return Outcome{}, err
	}

	fresh, _, err := runEffectiveChecks(ctx, e, artifact.Repositories, targets, stage, fix, ticketDir, newExecer(), cfg.Ops.ToolTimeout())
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

// dirCheckOptions are the directory form's own flags.
type dirCheckOptions struct {
	Fix                 bool
	Pack                string
	RequireInstructions bool
}

// runStandardsCheckDir is the ticket-free CI form: org+repo layers
// only (no team — there is no repo.json to name one), languages and
// OPS targets detected from dir itself, always stage "checks", no
// handoff write. Exits 6 invalid_result naming every open must finding
// — the phase-14 plan's design decision 9, settling the plan's own
// "Decides: standards check in the repositories' own CI" as yes,
// enforced here.
//
// It runs where CI runs: with no config.json it uses the built-in
// defaults (config init's template), --pack names the org layer
// explicitly, and a directory without AGENTS.md/CLAUDE.md is a warning
// unless --require-instructions — the instructions file exists to be
// injected into implement and review, which this form never runs.
func runStandardsCheckDir(ctx context.Context, f Flags, dir string, opts dirCheckOptions) (Outcome, error) {
	var warnings []string
	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		if f.Config != "" || !isMissingArtifact(err) {
			return Outcome{}, err
		}
		cfg = config.FromTemplate()
		warnings = append(warnings, fmt.Sprintf("no config.json at %s; using the built-in defaults", config.ResolvePath(f.Home, f.Config)))
	}

	orgPack, cleanup, err := resolvePackFlag(ctx, opts.Pack)
	defer cleanup()
	if err != nil {
		return Outcome{}, err
	}

	name, err := repoNameFromDir(dir)
	if err != nil {
		return Outcome{}, err
	}
	repos := []repoArtifactRepository{{Name: name, LocalPath: dir}}

	detected, err := ops.DetectAll(buildOpsRegistry(f, cfg).Addons(), []ops.DetectInput{{Repo: toDetectRepo(repos[0])}}, ops.ScopeRepo)
	if err != nil {
		return Outcome{}, err
	}

	e, packs, err := resolveRepos(f, cfg, resolveScope{
		Repos: repos, Addons: addonsOf(detected.Targets),
		OrgPack: orgPack, InstructionsOptional: !opts.RequireInstructions,
	})
	defer standards.ClosePacks(packs)
	if err != nil {
		return Outcome{}, err
	}
	for _, m := range e.MissingInstructions {
		warnings = append(warnings, m+" (pass --require-instructions to make this an error)")
	}

	// Every detected target, not just changed ones: there is no diff in
	// the directory form, and CI wants the whole repository checked.
	targets := checkTargets{Targets: detected.Targets}
	findings, skipped, err := runEffectiveChecks(ctx, e, repos, targets, "checks", opts.Fix, "", newExecer(), cfg.Ops.ToolTimeout())
	if err != nil {
		return Outcome{}, err
	}

	if open := openMustFindings(findings); len(open) > 0 {
		return Outcome{Warnings: warnings}, &envelope.Error{
			Code:    "invalid_result",
			Message: fmt.Sprintf("%s: %d open must finding(s)", dir, len(open)),
			Details: formatFindingDetails(open),
		}
	}

	res := toStandardsCheckResult("", dir, "checks", findings)
	res.Skipped = skipped
	return Outcome{Result: res, Warnings: warnings}, nil
}

// resolvePackFlag turns --pack into the org layer's location: an
// existing directory is used in place; <git-url>@<ref> is synced into a
// temporary home — never ~/.ai-flow and never the checked directory —
// which cleanup removes. An empty flag is no override.
func resolvePackFlag(ctx context.Context, pack string) (*standards.PackLocation, func(), error) {
	noop := func() {}
	if pack == "" {
		return nil, noop, nil
	}
	if info, err := os.Stat(pack); err == nil && info.IsDir() {
		return &standards.PackLocation{ID: dirPackID, Ref: "workdir", Dir: pack}, noop, nil
	}

	at := strings.LastIndex(pack, "@")
	if at <= 0 || at == len(pack)-1 || strings.Contains(pack[at+1:], ":") {
		return nil, noop, &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("--pack %q is neither an existing directory nor <git-url>@<ref>", pack),
		}
	}
	repo, ref := pack[:at], pack[at+1:]

	home, err := os.MkdirTemp("", "ai-flow-pack-")
	if err != nil {
		return nil, noop, fmt.Errorf("cli: create pack sync dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(home) }

	synced, err := standards.Sync(ctx, standards.SyncOptions{
		Home:        home,
		Sources:     []standards.Source{{ID: dirPackID, Repo: repo, Ref: ref, Layer: standards.LayerOrg}},
		GitHubToken: standards.GitHubTokenFromEnv(),
		Timeout:     standards.DefaultSyncTimeout,
	})
	if err != nil {
		return nil, cleanup, err
	}
	st := standards.StatusOf(home, dirPackID, ref, nil)
	return &standards.PackLocation{ID: dirPackID, Ref: ref, Repo: repo, Commit: st.Commit, Dir: synced[0].Path}, cleanup, nil
}

// dirPackID is the pack id --pack's org layer carries: findings name
// their layer, never this id, so it only appears in the resolution's
// own pack list.
const dirPackID = "pack"

// repoNameFromDir names the directory form's one repository after the
// directory itself — resolved to an absolute path first, so `.` names
// the checkout rather than becoming a repository called ".".
func repoNameFromDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("cli: resolve %s: %w", dir, err)
	}
	return filepath.Base(abs), nil
}

// checkTargets is the OPS scope addon-scoped ("policy"/"scan") checks
// run against: the targets, and — ticket form only — the rendering
// hash render-drift recorded per target, which `input: render` checks
// read.
type checkTargets struct {
	Targets      []ops.Target
	RenderHashes map[string]string
}

// ticketCheckTargets is the ticket form's scope: every target
// ops-targets.json marks changed (the same scope `ops drift`/`ops
// dry-run` use), with render-drift's hashes. Tolerant of either file
// being absent — an ordinary ticket with no OPS content at all simply
// has no target for an addon-scoped check to run against.
func ticketCheckTargets(ticketDir string) (checkTargets, error) {
	var out checkTargets
	targetsArt, err := loadOpsTargetsTolerant(ticketDir)
	if err != nil {
		return out, err
	}
	if targetsArt != nil {
		// changedOrOneTarget with no --target flag is exactly "every
		// target ops-targets.json marks changed".
		out.Targets, err = changedOrOneTarget(targetsArt.Targets, "")
		if err != nil {
			return out, err
		}
	}
	hashes, err := loadRenderDriftResultsTolerant(ticketDir)
	if err != nil {
		return out, err
	}
	out.RenderHashes = make(map[string]string, len(hashes))
	for key, t := range hashes {
		out.RenderHashes[key] = t.Hash
	}
	return out, nil
}

// runEffectiveChecks runs every effective, non-shadowed check that
// applies to stage, against every repo (or, for "policy"/"scan", every
// target in scope) it applies to. A waived check still runs — its
// findings are recorded with status "waived" rather than never
// appearing at all, so checks-results.json (and the output.md/PR body
// render built on it) shows what was found and accepted, not just that
// a waiver exists. secrets is standards.UnwaivableKind, so this path
// never actually rewrites its status in practice.
//
// A check whose tool is not installed reports one open finding per
// missing tool, at the check's own severity, instead of the empty
// report it would otherwise be indistinguishable from: a must check
// that could not run blocks exactly like one that failed.
//
// The second return value lists applicable addon-scoped checks that
// had nothing to run against (see runAddonScopedCheck).
func runEffectiveChecks(ctx context.Context, e *standards.Effective, repos []repoArtifactRepository, scope checkTargets, stage string, fix bool, ticketDir string, x exec.Execer, timeout time.Duration) ([]check.Finding, []skippedCheck, error) {
	// Detected once per repository, not once per (check, repository)
	// pair: a repository's own languages never change during one run.
	// One map keyed by repo name serves both the per-repo and the
	// addon-scoped loops below.
	repoInfo := make(map[string]repoCheckInfo, len(repos))
	for _, r := range repos {
		langs, err := detect.Languages(toDetectRepo(r))
		if err != nil {
			return nil, nil, err
		}
		repoInfo[r.Name] = repoCheckInfo{Langs: langs, LocalPath: r.LocalPath}
	}

	rec := &missingToolRecorder{Execer: x}
	var out []check.Finding
	var skipped []skippedCheck

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
			findings, err := rec.run(ctx, runner, check.RunInput{Check: c, Stage: stage, Fix: fix, TicketDir: ticketDir}, timeout, "", "")
			if err != nil {
				return nil, nil, err
			}
			out = append(out, decorateFindings(findings, c, stage)...)
			continue
		}

		if addonScopedKinds[c.Kind] {
			findings, skips, err := runAddonScopedCheck(ctx, c, scope, repoInfo, ticketDir, stage, fix, runner, rec, timeout)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, decorateFindings(findings, c, stage)...)
			skipped = append(skipped, skips...)
			continue
		}

		for _, r := range repos {
			if !standards.AppliesToAnyStage(c.Check.AppliesTo, repoInfo[r.Name].Langs, e.Addons) {
				continue
			}
			findings, err := rec.run(ctx, runner, check.RunInput{
				Check: c, Repo: check.Repo{Name: r.Name, LocalPath: r.LocalPath},
				Stage: stage, Fix: fix, TicketDir: ticketDir,
			}, timeout, r.Name, "")
			if err != nil {
				return nil, nil, err
			}
			out = append(out, decorateFindings(findings, c, stage)...)
		}
	}
	return out, skipped, nil
}

// missingToolRecorder is the Execer every check runner gets: it notes
// each tool the real Execer could not find on PATH, so run can turn
// "the tool never ran" into a finding rather than letting a runner's
// tool_failed tolerance read it as "the tool ran and found nothing".
type missingToolRecorder struct {
	exec.Execer
	missing []string
}

func (r *missingToolRecorder) Run(ctx context.Context, tool string, args []string, opts exec.RunOpts) (exec.Output, error) {
	out, err := r.Execer.Run(ctx, tool, args, opts)
	if missing, ok := exec.IsNotFound(err); ok {
		r.missing = append(r.missing, missing)
	}
	return out, err
}

// run calls runner with r as its Execer and appends one open finding
// per tool found missing during that call. A runner that propagated the
// not-found error itself (gofmt, gitleaks — they tolerate no tool
// failure) is handled the same way, so every runner reports a missing
// tool identically.
func (r *missingToolRecorder) run(ctx context.Context, runner check.CheckRunner, in check.RunInput, timeout time.Duration, repo, target string) ([]check.Finding, error) {
	r.missing = r.missing[:0]
	findings, err := runner.Run(ctx, in, r, timeout)
	if err != nil {
		if _, ok := exec.IsNotFound(err); !ok {
			return nil, err
		}
	}
	for _, tool := range strutil.DedupSorted(r.missing) {
		findings = append(findings, check.Finding{
			Tool: tool, Repo: repo, Target: target, Status: "open",
			Message: fmt.Sprintf("%s is not installed (not on PATH); check %s did not run", tool, in.Check.ID),
		})
	}
	return findings, nil
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
// target in scope whose addon c's own applies_to selects. Check.Input
// == "render" needs a rendering render-drift already wrote
// (render.RedactedDir over the hash render-drift-results.json recorded
// for that target); a target with no such hash (its addon has no
// Render/Validate capability, render-drift has not run this run, or
// this is the directory form, which renders nothing) is skipped —
// "nothing to check here yet," not a failure — and reported back so
// the directory form can say so instead of looking like a clean pass.
func runAddonScopedCheck(
	ctx context.Context, c standards.EffectiveCheck, scope checkTargets, repoInfo map[string]repoCheckInfo,
	ticketDir, stage string, fix bool, runner check.CheckRunner, rec *missingToolRecorder, timeout time.Duration,
) ([]check.Finding, []skippedCheck, error) {
	var out []check.Finding
	var skipped []skippedCheck
	for _, t := range scope.Targets {
		info, ok := repoInfo[t.Repo]
		if !ok {
			continue // repo.json names no such repository — nothing to check
		}
		if !standards.AppliesToAnyStage(c.Check.AppliesTo, info.Langs, []string{t.Addon}) {
			continue
		}

		// check.Target, not check.Repo: runPolicy/runScan (the only
		// runners addonScopedKinds ever dispatches to) read Target.Repo/
		// Target.SourceDir exclusively, never RunInput.Repo.
		target := check.Target{
			Key: ops.TargetKey(t), Addon: t.Addon, Repo: t.Repo,
			SourceDir: filepath.Join(info.LocalPath, t.Path),
		}
		if c.Input == "render" {
			hash := scope.RenderHashes[target.Key]
			if hash == "" {
				skipped = append(skipped, skippedCheck{CheckID: c.ID, Target: target.Key, Reason: "needs a rendering (input: render); none exists for this target"})
				continue
			}
			target.RenderDir = render.RedactedDir(ticketDir, t, hash)
		}

		findings, err := rec.run(ctx, runner, check.RunInput{
			Check: c, Target: target, Stage: stage, Fix: fix, TicketDir: ticketDir,
		}, timeout, t.Repo, target.Key)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, findings...)
	}
	return out, skipped, nil
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
