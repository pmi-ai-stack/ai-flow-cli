package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/render"
)

func newOpsTestCmd(cio cmdIO) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Detect, run and tear down the infra-test stage's harnesses",
	}
	cmd.AddCommand(newOpsTestDetectCmd(cio))
	cmd.AddCommand(newOpsTestRunCmd(cio))
	cmd.AddCommand(newOpsTestTeardownCmd(cio))
	return cmd
}

// targetHarness pairs one detected ops.Harness with the target it
// belongs to and the tool harness.ToolFor picked for it. A harness
// ToolFor does not recognize is silently dropped — this package only
// runs what it knows how to.
type targetHarness struct {
	Target  ops.Target
	Harness ops.Harness
	Tool    string
}

// harnessesForChangedTargets enumerates every runnable harness across
// every changed target in ticketDir's ops-targets.json — infra-test's
// own detection surface (plan design decision 1: this package holds no
// detection logic of its own, only dispatch). A ticket with no
// ops-targets.json yet, or none of whose targets are changed, reports
// an empty, non-nil slice — not an error, mirroring hasChangedTarget's
// own tolerant-of-absence shape. Also returns the repo.json it loaded
// along the way (nil on that same early-return path), so a caller that
// needs it too — today only newOpsTestRunCmd — does not load it a
// second time.
func harnessesForChangedTargets(f Flags, cfg *config.Config, key string) (*repoArtifact, []targetHarness, error) {
	ticketDir := handoff.TicketDir(f.Home, key)
	targetsArt, err := loadOpsTargetsTolerant(ticketDir)
	if err != nil || targetsArt == nil {
		return nil, nil, err
	}
	repoArt, err := loadRepoArtifact(ticketDir)
	if err != nil {
		return nil, nil, err
	}

	reg := buildOpsRegistry(f, cfg)
	out := []targetHarness{}
	for _, t := range targetsArt.Targets {
		if t.Changed == nil || !*t.Changed {
			continue
		}
		addon, ok := reg.Lookup(t.Addon)
		if !ok {
			continue
		}
		repo, ok := repoByName(repoArt, t.Repo)
		if !ok {
			continue
		}
		hs, err := addon.Harnesses(toDetectRepo(repo), t)
		if err != nil {
			return nil, nil, fmt.Errorf("ops test: harnesses for %s: %w", ops.TargetKey(t), err)
		}
		for _, h := range hs {
			tool, ok := harness.ToolFor(t.Addon, h)
			if !ok {
				continue
			}
			out = append(out, targetHarness{Target: t, Harness: h, Tool: tool})
		}
	}
	return repoArt, out, nil
}

// repoByName finds art's own repository named name.
func repoByName(art *repoArtifact, name string) (repoArtifactRepository, bool) {
	for _, r := range art.Repositories {
		if r.Name == name {
			return r, true
		}
	}
	return repoArtifactRepository{}, false
}

// hasHarness is `stage should-run KEY infra-test`'s own input (plan
// design decision 13): detection only, not gated on ops.e2e.enabled.
func hasHarness(f Flags, cfg *config.Config, key string) (bool, error) {
	_, ths, err := harnessesForChangedTargets(f, cfg, key)
	if err != nil {
		return false, err
	}
	return len(ths) > 0, nil
}

type opsTestDetectEntry struct {
	Harness string `json:"harness"`
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Tool    string `json:"tool"`
	WillRun bool   `json:"will_run"`
}

// harnessWillRun is the one place "would this harness actually run"
// is decided — shared by `ops test detect`'s own WillRun prediction
// and selectHarnesses' real selection, so the two can never silently
// disagree: every unit harness always runs; an e2e harness only when
// e2eEnabled (plan design decision 12's own gate, overridable per
// harness by selectHarnesses' own explicit --harness path, which does
// not call this function at all).
func harnessWillRun(kind ops.HarnessKind, e2eEnabled bool) bool {
	return kind == ops.HarnessUnit || e2eEnabled
}

// sortByTargetHarness sorts s in place by (Target, Harness) — the one
// ordering opsTestDetectEntry/infraTestEntry/opsTestRunEntry all need
// for byte-stable output.
func sortByTargetHarness[T any](s []T, target, harness func(T) string) {
	sort.Slice(s, func(i, j int) bool {
		if target(s[i]) != target(s[j]) {
			return target(s[i]) < target(s[j])
		}
		return harness(s[i]) < harness(s[j])
	})
}

type opsTestDetectResult struct {
	Harnesses []opsTestDetectEntry `json:"harnesses"`
	Key       string               `json:"key"`
}

func newOpsTestDetectCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "detect KEY",
		Short: "List every harness the infra-test stage would run, without running any of them",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops test detect", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			_, ths, err := harnessesForChangedTargets(f, cfg, key)
			if err != nil {
				return Outcome{}, err
			}

			entries := make([]opsTestDetectEntry, 0, len(ths))
			for _, th := range ths {
				entries = append(entries, opsTestDetectEntry{
					Harness: th.Harness.Name, Kind: string(th.Harness.Kind),
					Target: ops.TargetKey(th.Target), Tool: th.Tool,
					WillRun: harnessWillRun(th.Harness.Kind, cfg.Ops.E2E.Enabled),
				})
			}
			sortByTargetHarness(entries, func(e opsTestDetectEntry) string { return e.Target }, func(e opsTestDetectEntry) string { return e.Harness })
			return Outcome{Result: opsTestDetectResult{Key: key, Harnesses: entries}}, nil
		}),
	}
}

// infraTestEntry is infra-test-results.json's own per-(target,harness)
// shape — field order matches the JSON-key alphabetical order
// 07-determinism.md requires (ops.Target's own doc comment states the
// same rule; encoding/json preserves struct declaration order, it does
// not sort it).
type infraTestEntry struct {
	Failed   int      `json:"failed"`
	Failures []string `json:"failures"`
	Harness  string   `json:"harness"`
	Kind     string   `json:"kind"`
	Passed   int      `json:"passed"`
	Report   string   `json:"report"`
	Skipped  int      `json:"skipped"`
	Target   string   `json:"target"`
	Tool     string   `json:"tool"`
	Total    int      `json:"total"`
}

type infraTestArtifact struct {
	Targets []infraTestEntry `json:"targets"`
}

// mergeInfraTestEntries replaces, in existing, every entry whose own
// (Target, Harness) key also appears in updates, appends any
// updates entry with no existing match, and returns the result sorted
// by (Target, Harness) — the same per-key-replacement shape
// internal/standards/check's mergeFindings (phase 14) already
// established for a file more than one command call can touch over a
// ticket's lifetime (plan design decision 18).
func mergeInfraTestEntries(existing, updates []infraTestEntry) []infraTestEntry {
	byKey := map[string]infraTestEntry{}
	for _, e := range existing {
		byKey[e.Target+"\x00"+e.Harness] = e
	}
	for _, e := range updates {
		byKey[e.Target+"\x00"+e.Harness] = e
	}
	out := make([]infraTestEntry, 0, len(byKey))
	for _, e := range byKey {
		out = append(out, e)
	}
	sortByTargetHarness(out, func(e infraTestEntry) string { return e.Target }, func(e infraTestEntry) string { return e.Harness })
	return out
}

// loadInfraTestArtifactTolerant reads ticketDir's infra-test-results.json,
// (nil, nil) when it does not exist yet.
func loadInfraTestArtifactTolerant(ticketDir string) (*infraTestArtifact, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "infra-test-results.json"))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var a infraTestArtifact
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("cli: decode infra-test-results.json: %w", err)
	}
	return &a, nil
}

// reportFilename derives infra-test-reports/'s own filename for one
// (target, harness) pair — every non-alphanumeric character folded to
// "-" so the result is a single, safe path segment regardless of how
// many "/" or ":" the target key and harness path carry.
func reportFilename(target, harnessName string) string {
	safe := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
				b.WriteRune(r)
			default:
				b.WriteByte('-')
			}
		}
		return b.String()
	}
	return safe(target) + "__" + safe(harnessName) + ".log"
}

// relativeToTarget rewrites harnessName (repo-root-relative, per
// ops.Harness.Name's own doc comment) to targetPath-relative — the
// same "." special case ops.Target.Path can carry that
// internal/ops/addons/kubectl/detect.go's own filesUnder already
// handles.
func relativeToTarget(harnessName, targetPath string) string {
	if targetPath == "" || targetPath == "." {
		return harnessName
	}
	return strings.TrimPrefix(harnessName, targetPath+"/")
}

type opsTestRunEntry struct {
	Failed   int      `json:"failed"`
	Failures []string `json:"failures"`
	Harness  string   `json:"harness"`
	Kind     string   `json:"kind"`
	Passed   int      `json:"passed"`
	Ref      string   `json:"ref"`
	Report   string   `json:"report"`
	Skipped  int      `json:"skipped"`
	Target   string   `json:"target"`
	Tool     string   `json:"tool"`
	Total    int      `json:"total"`
}

type opsTestRunResult struct {
	Harnesses []opsTestRunEntry `json:"harnesses"`
	Key       string            `json:"key"`
	Ref       string            `json:"ref"`
}

func newOpsTestRunCmd(cio cmdIO) *cobra.Command {
	var harnessFlag, refFlag string
	cmd := &cobra.Command{
		Use:   "run KEY [--harness H] [--ref branch|base]",
		Short: "Run every eligible harness (or one named by --harness) and record the result",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops test run", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			ref, err := parseRef(refFlag)
			if err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())

			cfg, err := config.Load(f.Home, f.Config)
			if err != nil {
				return Outcome{}, err
			}
			ticketDir := handoff.TicketDir(f.Home, key)
			// repoArt comes back from harnessesForChangedTargets itself
			// (needed below, in runOneHarness) rather than a second,
			// separate loadRepoArtifact call: when ths is empty, repoArt
			// is nil too, but then the loop below never runs and so
			// never dereferences it.
			repoArt, ths, err := harnessesForChangedTargets(f, cfg, key)
			if err != nil {
				return Outcome{}, err
			}

			selected, err := selectHarnesses(ths, harnessFlag, cfg.Ops.E2E.Enabled)
			if err != nil {
				return Outcome{}, err
			}

			if !f.Simulate {
				if needsCluster(selected) {
					if err := harness.EnsureCluster(cmd.Context(), newOpsExecer(), f.Home, key, cfg.Ops.ToolTimeout()); err != nil {
						return Outcome{}, err
					}
				}
			}

			reg := buildOpsRegistry(f, cfg)
			// hashByTarget memoizes renderForTarget's own tree hash per
			// target for this invocation: a target with more than one
			// selected harness (e.g. a unit .tftest.hcl plus a Terratest
			// e2e one) renders and hashes its own source tree once, not
			// once per harness.
			hashByTarget := map[string]string{}
			entries := make([]opsTestRunEntry, 0, len(selected))
			artifactEntries := make([]infraTestEntry, 0, len(selected))
			for _, th := range selected {
				targetKey := ops.TargetKey(th.Target)
				hash, ok := hashByTarget[targetKey]
				if !ok {
					addon, _ := reg.Lookup(th.Target.Addon)
					hash, err = renderForTarget(cmd.Context(), f, key, repoArt, th.Target, ref, addon)
					if err != nil {
						return Outcome{}, err
					}
					hashByTarget[targetKey] = hash
				}
				entry, artEntry, err := runOneHarness(cmd.Context(), f, cfg, hash, key, ref, th)
				if err != nil {
					return Outcome{}, err
				}
				entries = append(entries, entry)
				if artEntry != nil {
					artifactEntries = append(artifactEntries, *artEntry)
				}
			}
			sortByTargetHarness(entries, func(e opsTestRunEntry) string { return e.Target }, func(e opsTestRunEntry) string { return e.Harness })

			// Only a --ref branch run is merged into infra-test-results.json
			// (plan design decision 10): a --ref base run's whole purpose is
			// to fail on purpose (RED), and must never make the stage's own
			// failure gate see an intentionally-red diagnostic entry.
			if !f.Simulate && ref == ops.RefBranch && len(artifactEntries) > 0 {
				existing, err := loadInfraTestArtifactTolerant(ticketDir)
				if err != nil {
					return Outcome{}, err
				}
				var prior []infraTestEntry
				if existing != nil {
					prior = existing.Targets
				}
				merged := mergeInfraTestEntries(prior, artifactEntries)
				if _, err := handoff.WriteArtifact(ticketDir, "infra-test-results.json", "infra-test-results", infraTestArtifact{Targets: merged}); err != nil {
					return Outcome{}, err
				}
			}

			return Outcome{Result: opsTestRunResult{Key: key, Ref: refFlag, Harnesses: entries}}, nil
		}),
	}
	cmd.Flags().StringVar(&harnessFlag, "harness", "", "run only the harness with this exact name (as reported by `ops test detect`)")
	cmd.Flags().StringVar(&refFlag, "ref", string(ops.RefBranch), "which rendering to run the harness against: branch or base")
	return cmd
}

// selectHarnesses narrows ths to harnessFlag's own match when set
// (honored regardless of e2eEnabled — an explicit --harness request is
// always run, plan design decision 12's own "override" reading), else
// to every unit harness plus every e2e harness only when e2eEnabled.
func selectHarnesses(ths []targetHarness, harnessFlag string, e2eEnabled bool) ([]targetHarness, error) {
	if harnessFlag != "" {
		for _, th := range ths {
			if th.Harness.Name == harnessFlag {
				return []targetHarness{th}, nil
			}
		}
		return nil, &envelope.Error{Code: "usage", Message: fmt.Sprintf("no harness %q detected", harnessFlag)}
	}
	out := []targetHarness{}
	for _, th := range ths {
		if harnessWillRun(th.Harness.Kind, e2eEnabled) {
			out = append(out, th)
		}
	}
	return out, nil
}

// needsCluster reports whether any of selected needs the ticket's own
// kind cluster (plan design decision 14).
func needsCluster(selected []targetHarness) bool {
	for _, th := range selected {
		if harness.NeedsCluster(th.Tool) {
			return true
		}
	}
	return false
}

// runOneHarness runs th's own harness against hash's real render-cache
// directory (plan design decision 8 — hash comes from the caller,
// which renders and memoizes per target once, not per harness: a
// target with more than one selected harness, e.g. a .tftest.hcl unit
// harness alongside a Terratest e2e one, must not re-render and
// re-hash its own source tree a second time), writes its raw output
// under infra-test-reports/, and returns both the envelope entry and —
// only for a unit harness or when the tool succeeded in producing a
// result at all — the artifact entry `ops test run`'s own caller
// persists.
func runOneHarness(ctx context.Context, f Flags, cfg *config.Config, hash string, key string, ref ops.Ref, th targetHarness) (opsTestRunEntry, *infraTestEntry, error) {
	cacheRoot := render.CachePath(f.Home, hash)
	relToTarget := relativeToTarget(th.Harness.Name, th.Target.Path)

	in := harness.RunInput{Tool: th.Tool, Timeout: cfg.Ops.ToolTimeout()}
	in.Dir, in.Filter = harness.ResolveDir(th.Tool, cacheRoot, relToTarget)
	if harness.NeedsCluster(th.Tool) {
		in.KubeContext = harness.KubeContext(key)
		in.Kubeconfig = harness.KubeconfigPath(f.Home, key)
	}

	res, err := harness.Run(ctx, newOpsExecer(), in)
	if err != nil {
		return opsTestRunEntry{}, nil, err
	}

	ticketDir := handoff.TicketDir(f.Home, key)
	reportPath, err := handoff.WriteFile(filepath.Join(ticketDir, "infra-test-reports"), reportFilename(ops.TargetKey(th.Target), th.Harness.Name), res.Raw)
	if err != nil {
		return opsTestRunEntry{}, nil, err
	}
	reportRel, err := filepath.Rel(ticketDir, reportPath)
	if err != nil {
		reportRel = reportPath
	}
	reportRel = filepath.ToSlash(reportRel)

	entry := opsTestRunEntry{
		Target: ops.TargetKey(th.Target), Harness: th.Harness.Name, Kind: string(th.Harness.Kind), Tool: res.Tool,
		Total: res.Total, Passed: res.Passed, Failed: res.Failed, Skipped: res.Skipped,
		Failures: emptyIfNil(res.Failures), Report: reportRel, Ref: string(ref),
	}
	artEntry := infraTestEntry{
		Target: entry.Target, Harness: entry.Harness, Kind: entry.Kind, Tool: entry.Tool,
		Total: entry.Total, Passed: entry.Passed, Failed: entry.Failed, Skipped: entry.Skipped,
		Failures: entry.Failures, Report: entry.Report,
	}
	return entry, &artEntry, nil
}

// checkNoInfraTestFailure is `stage complete KEY infra-test`'s own
// gate (plan design decision 11): refuses while any persisted
// target/harness entry has failed > 0. data is checkPrimaryArtifact's
// own already-read, already-schema-valid infra-test-results.json
// bytes.
func checkNoInfraTestFailure(data []byte) error {
	var artifact infraTestArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return fmt.Errorf("cli: decode infra-test-results.json: %w", err)
	}
	var failing []string
	for _, e := range artifact.Targets {
		if e.Failed > 0 {
			failing = append(failing, e.Target+" / "+e.Harness)
		}
	}
	if len(failing) == 0 {
		return nil
	}
	sort.Strings(failing)
	return &envelope.Error{
		Code:    "invalid_result",
		Message: "infra-test-results.json has one or more harnesses with a failing test",
		Details: failing,
	}
}

type opsTestTeardownResult struct {
	Key string `json:"key"`
}

// teardownInfraTestCluster is `ops test teardown`'s own body, shared
// verbatim with `stage complete KEY infra-test` (both pass and fail)
// and `stage reset KEY infra-test` in internal/cli/stage.go — plan
// design decision 15: unconditionally safe to call.
func teardownInfraTestCluster(ctx context.Context, f Flags, key string) error {
	cfg, err := config.Load(f.Home, f.Config)
	if err != nil {
		return err
	}
	return harness.TeardownCluster(ctx, newOpsExecer(), f.Home, key, cfg.Ops.ToolTimeout())
}

func newOpsTestTeardownCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "teardown KEY",
		Short: "Delete the ticket's own kind cluster, if one exists",
		Args:  cobra.ExactArgs(1),
		RunE: wrap("ops test teardown", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			key := args[0]
			if err := handoff.ValidateKey(key); err != nil {
				return Outcome{}, err
			}
			f := FlagsFrom(cmd.Context())
			if f.Simulate {
				return Outcome{Result: opsTestTeardownResult{Key: key}}, nil
			}
			if err := teardownInfraTestCluster(cmd.Context(), f, key); err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: opsTestTeardownResult{Key: key}}, nil
		}),
	}
}
