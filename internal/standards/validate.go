package standards

import (
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// renderingValidatorTools names the tools the spec says belong only to
// render-drift/dry-run (phases 20-27), never to a checks/ship-stage
// "lint" check — "Each tool has exactly one home." The Jenkins
// declarative linter's actual binary name is provisional (jenkins-lint)
// pending phase 27's own addon, mirroring how earlier phases flag a
// tool name chosen ahead of the phase that owns it for real.
var renderingValidatorTools = []string{"tflint", "kubeconform", "actionlint", "helm", "jenkins-lint"}

// Result is Validate's successful return value.
type Result struct {
	Manifest *Manifest
	Warnings []string
}

// Validate loads dir's manifest, checks every sections[].path and
// checks[].config exists under dir, and enforces the pack format's two
// structural rules: check_runner_missing (a check names a tool with no
// row in allowlist) and no lint check may name a rendering validator.
// allowedTools is normally exec.Table's own Tool column — a caller
// passes it explicitly (rather than this package reading exec.Table
// itself) so the two structural-rule tests can exercise both a minimal
// fixture allowlist and the real production one without a package-level
// test seam.
//
// A violation of check_runner_missing wins the returned error's Code
// over a generic pack_invalid whenever any violation of that rule
// exists, regardless of what else is wrong (see the phase-12 plan's
// design decision 7); Details always lists every violation found, in a
// stable, sorted order.
func Validate(dir string, allowedTools []string) (Result, error) {
	m, err := Load(dir)
	if err != nil {
		return Result{}, err
	}

	var violations []string
	var sawCheckRunnerMissing bool

	root, err := os.OpenRoot(dir)
	if err != nil {
		return Result{}, fmt.Errorf("standards: open %s: %w", dir, err)
	}
	defer root.Close() //nolint:errcheck // read-only Root, nothing left to flush

	for _, s := range m.Sections {
		if _, err := root.Stat(s.Path); err != nil {
			violations = append(violations, fmt.Sprintf("section %s: path %q does not exist under %s", s.ID, s.Path, dir))
		}
	}

	for _, c := range m.Checks {
		if c.Config != "" {
			if _, err := root.Stat(c.Config); err != nil {
				violations = append(violations, fmt.Sprintf("check %s: config %q does not exist under %s", c.ID, c.Config, dir))
			}
		}
		if !slices.Contains(allowedTools, c.Tool) {
			sawCheckRunnerMissing = true
			violations = append(violations, fmt.Sprintf("check %s: tool %q has no internal/ops/exec allowlist row", c.ID, c.Tool))
		}
		if c.Kind == "lint" && slices.Contains(renderingValidatorTools, c.Tool) {
			violations = append(violations, fmt.Sprintf("check %s: kind \"lint\" names rendering validator %q, which belongs to render-drift/dry-run", c.ID, c.Tool))
		}
	}

	if len(violations) == 0 {
		return Result{Manifest: m}, nil
	}

	sort.Strings(violations)
	code := "pack_invalid"
	if sawCheckRunnerMissing {
		code = "check_runner_missing"
	}
	return Result{}, &envelope.Error{
		Code:    code,
		Message: fmt.Sprintf("%s: %d violation(s)", dir, len(violations)),
		Details: violations,
	}
}

// ValidateAgainstProductionAllowlist is Validate with exec.Table's own
// Tool column — the shape every real caller (the CLI command, doctor)
// wants; tests use Validate directly against a minimal fixture table.
func ValidateAgainstProductionAllowlist(dir string) (Result, error) {
	return Validate(dir, allowlistTools(exec.Table))
}

func allowlistTools(rows []exec.Row) []string {
	seen := make(map[string]bool, len(rows))
	tools := make([]string, 0, len(rows))
	for _, r := range rows {
		if !seen[r.Tool] {
			seen[r.Tool] = true
			tools = append(tools, r.Tool)
		}
	}
	return tools
}
