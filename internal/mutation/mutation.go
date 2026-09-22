// Package mutation defines the tool-independent shape phase 7's four
// mutation-testing integrations share: internal/mutation/gremlins,
// stryker, mutmut and pitest each implement Tool, exactly the way
// internal/tracker/jira implements internal/tracker.Tracker and
// internal/host/github implements internal/host.Host. This package
// never imports any of the four — internal/cli picks the concrete
// implementation by the tool name internal/detect.Mutation already
// returned (see the plan's design decision 2).
package mutation

import (
	"context"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Scope is one repository's changed-code scope: the diff-derived files
// ScopeFor judged mutable for the tool in question — the plan's design
// decision 3 (mutation scope's own read-only result, and mutation
// run's internal input, computed by the same function).
type Scope struct {
	Repo  string
	Files []string
}

// Survivor is one mutant a tool's report marked as not killed by the
// suite — the only mutant kind mutation summarize needs to name
// individually, since it is the only one --equivalent can mark.
type Survivor struct {
	ID   string
	File string
	Line int
}

// Report is the normalized shape every Tool.Run produces: how many
// mutants the tool considered in total, how many the suite killed, and
// which ones survived. Total - Killed == len(Survivors) always holds —
// every implementation's parser is responsible for that invariant.
type Report struct {
	Total     int
	Killed    int
	Survivors []Survivor
}

// Tool is one mutation-testing provider. Mirrors verify.TestRunner's
// own shape (context, the thing to run, a Scope, an Execer, a
// timeout), minus Detect: internal/detect.Mutation already picks the
// tool name per repository, so a Tool never self-detects (design
// decision 1).
type Tool interface {
	Run(ctx context.Context, repo detect.Repo, scope Scope, x exec.Execer, timeout time.Duration) (Report, error)
}
