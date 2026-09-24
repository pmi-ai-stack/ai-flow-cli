// Package harness runs one already-detected ops.Harness (see
// internal/ops.Addon.Harnesses, implemented by phases 21/22's real
// addons for detection only) and normalizes its result — the
// infra-test stage's own engine, phase 24's own line in
// implementation-plan.md's Lane E: "harness detection per addon;
// runners and parsers for terraform test, Terratest, kuttl and
// chainsaw with recorded reports." This package owns no detection
// logic of its own (that stays inside each Addon.Harnesses, phases
// 21/22's own package) and no artifact-writing logic (that is
// internal/cli/ops_infratest.go's job, mirroring internal/mutation's
// own "pure package, internal/cli owns the file") — it only answers
// "given this harness, which tool runs it, and what did that tool
// report."
package harness

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Result is the normalized shape every one of this package's four
// runners produces — infra-test-results.json's own per-target-per-
// harness entry, before internal/cli adds the Target/Harness/Kind
// fields the caller already knows without this package's help.
// Skipped is folded from whichever tool actually reports one (today
// only terraform test does); a tool with no such concept always
// reports 0, never omits the field, so the schema stays uniform.
type Result struct {
	Tool     string
	Total    int
	Passed   int
	Failed   int
	Skipped  int
	Failures []string
	// Raw is the tool's own captured stdout (already redacted by
	// internal/ops/exec.Runner before it ever reaches this package) —
	// internal/cli writes it verbatim under infra-test-reports/.
	Raw []byte
}

// RunInput is one harness execution: which tool to run (from ToolFor),
// where, and — for the two tools that need a live cluster — how to
// reach it. Dir is always the target's real render-cache directory
// (never the redacted copy, never the raw git checkout — see the
// phase-24 plan's design decision 8): the one place in this codebase a
// tool is ever pointed at unredacted content on purpose, the same
// restriction Addon.Validate/Drift/DryRun already observe.
type RunInput struct {
	Tool string
	Dir  string
	// Filter is terraform-test's own -filter value: the harness file's
	// own name relative to Dir (never Dir itself, which callers must
	// already have resolved to the target's own cache root).
	Filter string
	// KubeContext/Kubeconfig are set only for kuttl/chainsaw — see
	// kind.go's own KubeContext/KubeconfigPath.
	KubeContext string
	Kubeconfig  string
	Timeout     time.Duration
}

// ToolFor is this package's one dispatch point (plan design decision
// 2): which of the four tools runs a given addon's harness. Keyed off
// the harness's own file name for the two Kubernetes-family tools
// rather than a hardcoded addon name, so a future addon that reuses
// kubectl's own kuttl-test.yaml/chainsaw-test.yaml convention (helm,
// kustomize — phase 26's own "as kubectl" capability-matrix row) needs
// no change here.
func ToolFor(addonName string, h ops.Harness) (tool string, ok bool) {
	switch {
	case addonName == "terraform" && h.Kind == ops.HarnessUnit:
		return "terraform-test", true
	case addonName == "terraform" && h.Kind == ops.HarnessE2E:
		return "terratest", true
	case h.Kind == ops.HarnessE2E && strings.HasSuffix(h.Name, "kuttl-test.yaml"):
		return "kuttl", true
	case h.Kind == ops.HarnessE2E && strings.HasSuffix(h.Name, "chainsaw-test.yaml"):
		return "chainsaw", true
	default:
		return "", false
	}
}

// ResolveDir builds RunInput's own Dir/Filter split for tool, given the
// target's real render-cache root and the harness file's own path
// relative to that target (internal/cli's relativeToTarget). This is
// each runner's own invocation convention (terraformtest.go's runner
// uses -filter against the cache root itself; every other runner in
// this package runs in the directory containing the harness file, its
// own suite/package directory) — kept here, next to ToolFor and
// NeedsCluster, rather than in the CLI layer, so a future tool's own
// directory-vs-file convention is a one-package change.
func ResolveDir(tool, cacheRoot, relToTarget string) (dir, filter string) {
	if tool == "terraform-test" {
		return cacheRoot, relToTarget
	}
	return filepath.Join(cacheRoot, filepath.Dir(filepath.FromSlash(relToTarget))), ""
}

// NeedsCluster reports whether tool requires the ticket's own kind
// cluster (plan design decision 14: created only when a selected
// harness this invocation is about to run actually needs one).
func NeedsCluster(tool string) bool {
	return tool == "kuttl" || tool == "chainsaw"
}

// Run dispatches to one of the four tool-specific runners named by
// in.Tool. kuttl and chainsaw share one shape (`<tool> test <suite-dir>
// --context <kind context>`, see textscan.go's own doc comment) closely
// enough that no separate per-tool wrapper is needed — in.Tool is
// itself the exact argv[0]/first-positional-arg-value both need. An
// unknown tool is a programmer error (ToolFor is this package's only
// producer of a Tool value) — not a condition a caller recovers from,
// so it is reported the same way as the other impossible-by-construction
// paths elsewhere in this codebase.
func Run(ctx context.Context, x exec.Execer, in RunInput) (Result, error) {
	switch in.Tool {
	case "terraform-test":
		return runTerraformTest(ctx, x, in)
	case "terratest":
		return runTerratest(ctx, x, in)
	case "kuttl", "chainsaw":
		return runTextScanTool(ctx, x, in.Tool, []string{"test", in.Dir, "--context", in.KubeContext}, in)
	default:
		return Result{}, fmt.Errorf("harness: unknown tool %q", in.Tool)
	}
}
