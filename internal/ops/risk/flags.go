package risk

import (
	"slices"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// dataLikeKindClass reports whether k is one of the four kind classes
// the protected formula's first disjunct names — the same four
// terraform's and kubectl's own Rollback implementations already use
// for their best-effort NonRevertible classification.
func dataLikeKindClass(k ops.KindClass) bool {
	switch k {
	case ops.KindData, ops.KindIAM, ops.KindSecret, ops.KindCRDOrNamespace:
		return true
	default:
		return false
	}
}

// ActionProtected implements the spec's own protected formula:
//
//	protected = (action ∈ {delete, replace} ∧ kind_class ∈ {data, iam, secret, crd_or_namespace})
//	          ∨ (action ≠ noop ∧ tier = prod ∧ kind ∈ ops.protected_kinds)
//
// A noop never makes an action protected — the spec's own words, and
// the property stage skip KEY impact's refusal and the gate's second
// trigger both depend on. protectedKinds is config.ops.protected_kinds
// verbatim.
func ActionProtected(a ops.Action, protectedKinds []string) bool {
	if (a.Action == ops.ActionDelete || a.Action == ops.ActionReplace) && dataLikeKindClass(a.KindClass) {
		return true
	}
	return a.Action != ops.ActionNoop && a.Tier == "prod" && slices.Contains(protectedKinds, a.Kind)
}

// ActionSensitive implements the spec's own sensitive formula:
// kind_class = secret, ORed into whatever the addon itself already
// observed (terraform's own before_sensitive/after_sensitive plan
// markers; kubectl always contributes false here, having no
// finer-grained signal — see the addon's own phase-22 design decision
// 5). Never discards the addon's own signal.
func ActionSensitive(a ops.Action) bool {
	return a.Sensitive || a.KindClass == ops.KindSecret
}
