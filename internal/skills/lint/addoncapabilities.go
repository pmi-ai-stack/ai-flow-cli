package lint

import (
	"strings"
	"sync"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/githubactions"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/helm"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/jenkins"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/kubectl"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/kustomize"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/addons/terraform"
)

// addonRegistry builds the same six-addon roster internal/cli/ops.go's
// buildOpsRegistry constructs for the shipped binary's own OPS registry.
// internal/skills/lint cannot import internal/cli (the reverse import
// already exists, via internal/cli/skills.go), so this is the identical
// "construct one layer up from internal/ops" shape, duplicated here
// rather than shared, for lint instead of execution — and, being a
// *ops.Registry rather than a hand-built map, it reuses Registry.Lookup
// for the tool-name lookup checkAddonSectionCapabilities needs, instead
// of re-deriving that operation too.
//
// Every addon's Capabilities() method (checked by reading all six
// addon.go files directly) is a fixed struct literal that reads no
// receiver field, so constructing each with a nil Execer and a nil
// *config.Config is safe: no subprocess runs, no config is read, only
// Name()/Capabilities() are ever called on the result.
var addonRegistry = sync.OnceValue(func() *ops.Registry {
	return ops.NewRegistry(
		terraform.New(nil, nil, ""),
		kubectl.New(nil, nil),
		helm.New(nil, nil),
		kustomize.New(nil, nil),
		githubactions.New(nil, nil),
		jenkins.New(nil, nil),
	)
})

// sectionRequired reports whether section is required content (true) or
// must be the one-line not_applicable form (false) for an addon with
// caps — the spec's own addon-section table: Impact is always required;
// every other section is required exactly when its own named capability
// is set. The five cases exhaust addonSections, this function's only
// caller's only input, so no other section name is ever reachable.
func sectionRequired(caps ops.Capabilities, section string) bool {
	switch section {
	case "Drift":
		return caps.Drift
	case "Changeset":
		return caps.DryRun
	case "Impact":
		return true
	case "Rollback":
		return caps.Rollback
	case "Testing":
		return caps.UnitTest || caps.E2ETest
	}
	return false
}

// notApplicablePrefix is the prose form the spec's own words ("one
// line, not_applicable, and why") take in practice — established by the
// phase-10 fixture (testdata/skills/valid/skills/addon-example-tool's
// own Rollback section: "Not applicable — ..."), not the literal JSON
// enum token used elsewhere for drift/dry-run results.
const notApplicablePrefix = "not applicable"

// isMarkedNotApplicable reports whether body (a section's own content)
// is the one-line not_applicable form: trim whitespace and any leading
// Markdown emphasis markers, then compare case-insensitively.
func isMarkedNotApplicable(body string) bool {
	trimmed := strings.TrimSpace(body)
	trimmed = strings.TrimLeft(trimmed, "*_")
	return strings.HasPrefix(strings.ToLower(trimmed), notApplicablePrefix)
}
