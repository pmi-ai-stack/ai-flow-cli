package terraform

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

// planJSON is a typed subset of "terraform show -json"'s own plan
// format — only what buildChangeset needs (format_version and
// terraform_version, the two fields every real plan carries first,
// are deliberately not modeled: this addon does not brand its output
// by them).
type planJSON struct {
	ResourceChanges []resourceChange `json:"resource_changes"`
}

type resourceChange struct {
	Address string `json:"address"`
	Mode    string `json:"mode"`
	Type    string `json:"type"`
	Change  change `json:"change"`
}

type change struct {
	Actions         []string        `json:"actions"`
	Before          map[string]any  `json:"before"`
	After           map[string]any  `json:"after"`
	BeforeSensitive json.RawMessage `json:"before_sensitive"`
	AfterSensitive  json.RawMessage `json:"after_sensitive"`
}

// dataMode is resourceChange.Mode's own value for a data source read —
// never an infrastructure change, always skipped by buildChangeset.
const dataMode = "data"

func parsePlanJSON(data []byte) (*planJSON, error) {
	var p planJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("terraform: decode plan JSON: %w", err)
	}
	return &p, nil
}

// actionKinds maps terraform's own documented change.actions
// combinations to ops.ActionKind. An unrecognized combination is a
// decode error in buildChangeset, never a silent guess — the phase-21
// plan's own §1.2.4: misclassifying a change would feed phase 23's
// risk scoring a wrong answer, which is worse than a loud failure.
// "read" (a data-source's own action) is deliberately absent: both
// callers (buildChangeset, driftResources) already skip every
// mode == dataMode resource before ever calling actionKind, so a
// managed resource — the only kind reaching this map — never carries
// it.
var actionKinds = map[string]ops.ActionKind{
	"no-op":         ops.ActionNoop,
	"create":        ops.ActionCreate,
	"update":        ops.ActionUpdate,
	"delete":        ops.ActionDelete,
	"delete,create": ops.ActionReplace,
	"create,delete": ops.ActionReplace,
}

// buildChangeset normalizes plan into an ops.Changeset for t/env:
// skips data-source reads, maps every resource_changes[] entry's
// Actions via actionKinds, computes Kind/KindClass from
// classifyKind(Type), FieldsChanged as the sorted, deduplicated set of
// top-level Before/After keys whose values differ (empty for
// create/delete/noop — the plan's own §1.2.3 scope line), Sensitive
// from the plan's own before_sensitive/after_sensitive markers.
// Protected is always false (§0.6/design decision 3 — phase 23's
// internal/ops/risk owns it). Actions are sorted by Address for byte
// stability, mirroring internal/cli/ops_dryrun.go's own
// normalizeChangeset.
func buildChangeset(plan *planJSON, t ops.Target, env ops.Env) (ops.Changeset, error) {
	var actions []ops.Action
	for _, rc := range plan.ResourceChanges {
		if rc.Mode == dataMode {
			continue
		}
		kind, err := actionKind(rc.Change.Actions)
		if err != nil {
			return ops.Changeset{}, fmt.Errorf("terraform: resource %s: %w", rc.Address, err)
		}

		var fieldsChanged []string
		if kind == ops.ActionUpdate || kind == ops.ActionReplace {
			fieldsChanged = changedFields(rc.Change.Before, rc.Change.After)
		}

		actions = append(actions, ops.Action{
			Action:        kind,
			Address:       rc.Address,
			Addon:         "terraform",
			Env:           env.Name,
			FieldsChanged: fieldsChanged,
			Kind:          rc.Type,
			KindClass:     classifyKind(rc.Type),
			Name:          rc.Address,
			Protected:     false,
			Sensitive:     anySensitive(rc.Change.BeforeSensitive) || anySensitive(rc.Change.AfterSensitive),
			Target:        ops.TargetKey(t),
			Tier:          env.Tier,
		})
	}

	sort.SliceStable(actions, func(i, j int) bool { return actions[i].Address < actions[j].Address })
	return ops.Changeset{Actions: actions}, nil
}

func actionKind(rawActions []string) (ops.ActionKind, error) {
	key := strings.Join(rawActions, ",")
	kind, ok := actionKinds[key]
	if !ok {
		return "", fmt.Errorf("unrecognized change.actions %v", rawActions)
	}
	return kind, nil
}

// changedFields returns the sorted, deduplicated top-level keys
// present in before or after whose values differ (a key missing from
// one side and present in the other counts as changed). Values
// themselves are never returned — only names — so no plan value ever
// leaves this package (the phase-21 plan's own design decision 4).
func changedFields(before, after map[string]any) []string {
	seen := map[string]bool{}
	for k := range before {
		seen[k] = true
	}
	for k := range after {
		seen[k] = true
	}

	var changed []string
	for k := range seen {
		bv, bok := before[k]
		av, aok := after[k]
		if bok != aok || !equalJSON(bv, av) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed
}

func equalJSON(a, b any) bool {
	ab, aErr := json.Marshal(a)
	bb, bErr := json.Marshal(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return string(ab) == string(bb)
}

// anySensitive reports whether raw (before_sensitive or
// after_sensitive, either a bare bool for a scalar attribute or a
// nested map/array of bools mirroring a structured value's own shape —
// terraform's own documented plan-JSON convention) contains any true
// leaf. Empty/absent raw is not sensitive.
func anySensitive(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return sensitiveLeaf(v)
}

func sensitiveLeaf(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case map[string]any:
		for _, sub := range t {
			if sensitiveLeaf(sub) {
				return true
			}
		}
	case []any:
		for _, sub := range t {
			if sensitiveLeaf(sub) {
				return true
			}
		}
	}
	return false
}
