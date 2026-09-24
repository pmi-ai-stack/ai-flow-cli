package jenkins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// Validate runs the declarative linter, but only for a Jenkinsfile
// target — a raw job-config XML or a Job DSL groovy script is not
// Declarative Pipeline syntax, so jenkins-cli's own "declarative-linter"
// subcommand has nothing to validate for either (§1.1.12 of the
// phase-27 plan). Reflects on r's own rendered file name (soleFile) to
// decide, rather than needing the Target this method's own frozen
// signature does not receive.
//
// No Jenkins binding anywhere in config (jenkinsBinding, env.go) is a
// soft, should-severity Finding — never an error: Addon.Validate has
// no Env parameter to resolve a target-specific binding from
// (doc.go's own paragraph), so a ticket that never configured a
// Jenkins environment at all must not have its render-drift stage fail
// over a tool this addon cannot honestly reach. A configured binding
// that fails for real (jenkins-cli itself errors) is a genuine
// tool_failed, propagated unchanged — this call's shape and its
// stdin-based input are provisional (doc.go), unverified against a
// real Jenkins server.
func (a *Addon) Validate(ctx context.Context, r *ops.Rendered) ([]ops.Finding, error) {
	name, err := soleFile(r.Dir)
	if err != nil {
		return nil, err
	}
	if name != "Jenkinsfile" {
		return nil, nil
	}

	url, ok := jenkinsBinding(a.cfg)
	if !ok {
		return []ops.Finding{{
			Severity: "should", Tool: "jenkins-cli",
			Message: "no jenkins binding configured in config.ops.environments; declarative-linter skipped",
		}}, nil
	}

	data, err := os.ReadFile(filepath.Join(r.Dir, name)) //nolint:gosec // G304: r.Dir is this addon's own rendering directory
	if err != nil {
		return nil, fmt.Errorf("jenkins: validate: %w", err)
	}

	args := append([]string{"declarative-linter", "-s", url}, jenkinsAuthArgs()...)
	out, err := a.x.Run(ctx, "jenkins-cli", args, exec.RunOpts{Dir: r.Dir, Stdin: string(data), Timeout: a.timeout()})
	if err == nil {
		return nil, nil
	}
	if !exec.IsToolFailed(err) {
		return nil, err
	}
	msg := strings.TrimSpace(out.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(out.Stdout)
	}
	return []ops.Finding{{Severity: "error", Tool: "jenkins-cli", Message: msg}}, nil
}
