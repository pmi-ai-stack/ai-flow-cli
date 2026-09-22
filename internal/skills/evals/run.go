package evals

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// ScenarioResult is one scenario's outcome.
type ScenarioResult struct {
	Skill        string       `json:"skill"`
	Scenario     ScenarioKind `json:"scenario"`
	Skipped      bool         `json:"skipped"`
	SkipReason   string       `json:"skip_reason,omitempty"`
	GraderPass   bool         `json:"grader_pass"`
	GraderReason string       `json:"grader_reason,omitempty"`
	OrderOK      bool         `json:"order_ok"`
	OrderWant    []string     `json:"order_want,omitempty"`
	OrderGot     []string     `json:"order_got,omitempty"`
	Pass         bool         `json:"pass"`
	Err          string       `json:"error,omitempty"`
}

// RunScenario runs one scenario end to end: extract the ticket key
// from its query (ExtractKey), stage a fresh Workspace under its own
// subdirectory of workRoot, stage the fixture and every skill under
// pluginRoot into it, run the named host, grade the transcript, check
// events.jsonl's call order, and report one ScenarioResult. The
// scenario's own temporary directory is removed before returning,
// success or failure — a caller driving many scenarios in one `skills
// evals` invocation must not accumulate one live workspace per
// scenario.
func RunScenario(ctx context.Context, x exec.UnlistedRunner, workRoot, pluginRoot string, skill skills.Skill, s Scenario, host, model string) ScenarioResult {
	res := ScenarioResult{Skill: skill.Name, Scenario: s.Kind}

	key, ok := ExtractKey(s.Query)
	if !ok {
		res.Err = fmt.Sprintf("no ticket key found in query %q", s.Query)
		return res
	}

	root, err := os.MkdirTemp(workRoot, "scenario-*")
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer func() { _ = os.RemoveAll(root) }()

	w, err := NewWorkspace(root, key)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	if err := w.StageFixture(filepath.Join(skill.Dir, "evals", s.Fixture)); err != nil {
		res.Err = err.Error()
		return res
	}
	if err := w.StageSkills(pluginRoot); err != nil {
		res.Err = err.Error()
		return res
	}

	runner, ok := Runners[host]
	if !ok {
		res.Err = fmt.Sprintf("unknown host %q", host)
		return res
	}

	transcript, err := runner.Run(ctx, x, w, s.Query, model)
	if err != nil {
		if errors.Is(err, ErrHostUnverified) {
			res.Skipped = true
			res.SkipReason = err.Error()
			return res
		}
		res.Err = err.Error()
		return res
	}

	grade, err := Grade(ctx, x, w, model, s, transcript)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	res.GraderPass = grade.Pass
	res.GraderReason = grade.Reason

	res.OrderWant = ExpectedCommandOrder(s.ExpectedBehavior)
	got, err := ReadCommandOrder(w.EventsPath())
	if err != nil {
		res.Err = err.Error()
		return res
	}
	res.OrderGot = got
	res.OrderOK = IsSubsequence(res.OrderWant, res.OrderGot)

	res.Pass = grade.Pass && res.OrderOK
	return res
}
