package exec

import (
	"fmt"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// CommandHints maps a tool name to the ai-flow command that performs the
// equivalent operation, named in the exec_not_allowed denial. Empty in
// phase 1; every later phase that adds a Row for a tool adds its hint
// here in the same PR.
var CommandHints = map[string]string{
	"git": "git prepare|checkout-base|checkout-branch|diff|commit|push KEY — never call git directly",
}

// NotAllowed builds the exec_not_allowed error (exit 2) for argv, which
// matched no allowlist row. The message names the equivalent ai-flow
// command when CommandHints has one for argv[0].
func NotAllowed(argv []string) *envelope.Error {
	shown := strings.Join(argv, " ")
	msg := fmt.Sprintf("%q is not on the ai-flow execution allowlist", shown)
	if len(argv) > 0 {
		if hint, ok := CommandHints[argv[0]]; ok {
			msg = fmt.Sprintf("%s; use `%s` instead", msg, hint)
		}
	}
	return &envelope.Error{Code: "exec_not_allowed", Message: msg}
}
