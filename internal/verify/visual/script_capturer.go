package visual

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// ScriptCapturer runs a project-provided command to take one
// screenshot — the plan's design decision 5: the command receives
// AI_FLOW_CAPTURE_{URL,VIEWPORT,OUT} as environment, never argv, so
// the project's own script never needs to track ai-flow's own flag
// names across versions.
type ScriptCapturer struct {
	Runner  exec.UnlistedRunner
	Command []string
	Dir     string
	Timeout time.Duration
}

var _ Capturer = (*ScriptCapturer)(nil)

// Capture runs c.Command via Runner.RunUnlisted (design decision 9's
// unlisted path — the capture script is, by this phase's own
// extension, the second of the two processes the Execution allowlist
// names as sitting outside the table) and verifies req.OutputPath
// exists afterward: a script that exits 0 without writing anything is
// a silent contract violation, reported as tool_failed rather than
// returned as if it had succeeded.
func (c *ScriptCapturer) Capture(ctx context.Context, req CaptureRequest) (CaptureResult, error) {
	if len(c.Command) == 0 {
		return CaptureResult{}, &envelope.Error{Code: "tool_missing", Message: "no capture_command configured in .ai-flow/visual.json"}
	}
	env := []string{
		"AI_FLOW_CAPTURE_URL=" + req.URL,
		"AI_FLOW_CAPTURE_VIEWPORT=" + req.Viewport,
		"AI_FLOW_CAPTURE_OUT=" + req.OutputPath,
	}
	_, err := c.Runner.RunUnlisted(ctx, c.Dir, c.Command[0], c.Command[1:], env, c.Timeout)
	if err != nil {
		return CaptureResult{}, err
	}
	if _, statErr := os.Stat(req.OutputPath); statErr != nil {
		return CaptureResult{}, &envelope.Error{
			Code:    "tool_failed",
			Message: fmt.Sprintf("capture_command exited 0 but did not write %s", req.OutputPath),
		}
	}
	return CaptureResult{Path: req.OutputPath}, nil
}
