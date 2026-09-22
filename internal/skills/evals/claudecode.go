package evals

import (
	"context"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// claudeCodeRunner runs one headless Claude Code session:
// `claude -p <query> --model <model> --output-format json`, cwd
// w.ProjectDir (where StageSkills installed .claude/skills/), HOME
// overridden to w.HomeDir so the session's own shelled-out `ai-flow`
// calls resolve --home's default into the staged fixture (design
// decision 2) — everything else (ANTHROPIC_API_KEY included) is
// inherited from the real process environment, since RunUnlisted only
// appends to os.Environ(), never replaces it.
type claudeCodeRunner struct{}

var _ HostRunner = claudeCodeRunner{}

func (claudeCodeRunner) Run(ctx context.Context, x exec.UnlistedRunner, w *Workspace, query, model string) (Transcript, error) {
	return runClaude(ctx, x, w, query, model)
}

// runClaude is claudeCodeRunner.Run's own body, factored out so
// Grade (which needs the identical subprocess shape for its own
// grading prompt) can call it without going through the HostRunner
// interface — see the plan's design decision 5.
func runClaude(ctx context.Context, x exec.UnlistedRunner, w *Workspace, prompt, model string) (Transcript, error) {
	args := []string{"-p", prompt, "--model", model, "--output-format", "json"}
	env := []string{"HOME=" + w.HomeDir}

	out, err := x.RunUnlisted(ctx, w.ProjectDir, "claude", args, env, hostTimeout)
	t := Transcript{Text: out.Stdout, ExitCode: out.ExitCode}
	if err != nil && out.ExitCode == 0 {
		// RunUnlisted's own error wraps any failure to start or run the
		// process at all; a non-zero ExitCode alongside it means the
		// process did start and did produce output — a session that
		// exited non-zero (a scenario the model correctly refused, for
		// instance) still produced a transcript worth grading, the
		// same reasoning exec.Output's own doc comment gives for the
		// allowlisted Run path's tool_failed case. Only a truly
		// unstarted process (no exit code at all) is this function's
		// own error.
		return t, err
	}
	return t, nil
}
