package exectest

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// RequireToolsEnv names the environment variable a job sets to the
// comma-separated tools it provisions (the nightly real-tool job does).
// A real-tool test that would skip because one of those tools is
// missing fails instead: a broken install must turn the night red, not
// green with every test skipped.
const RequireToolsEnv = "AI_FLOW_REQUIRE_TOOLS"

// RequireTool returns tool's path, or skips tb when tool is not on
// PATH — unless RequireToolsEnv names it, in which case tb fails.
// Every real-tool (-tags ops) test asks for its binaries through this.
func RequireTool(tb testing.TB, tool string) string {
	tb.Helper()
	if path, ok := exec.Which(tool); ok {
		return path
	}
	if toolRequired(tool) {
		tb.Fatalf("%s is not on PATH and %s names it", tool, RequireToolsEnv)
		return ""
	}
	tb.Skipf("%s not on PATH", tool)
	return ""
}

// HasTool is RequireTool for an optional half of a test: false (with a
// log line) when tool is absent, so the rest of the test still runs —
// and still a failure when RequireToolsEnv names tool.
func HasTool(tb testing.TB, tool string) bool {
	tb.Helper()
	if _, ok := exec.Which(tool); ok {
		return true
	}
	if toolRequired(tool) {
		tb.Fatalf("%s is not on PATH and %s names it", tool, RequireToolsEnv)
		return false
	}
	tb.Logf("%s not on PATH; the part of this test that needs it is skipped", tool)
	return false
}

func toolRequired(tool string) bool {
	var names []string
	for name := range strings.SplitSeq(os.Getenv(RequireToolsEnv), ",") {
		names = append(names, strings.TrimSpace(name))
	}
	return slices.Contains(names, tool)
}
