package ops

// ActionKind is one changeset action's own kind.
type ActionKind string

const (
	ActionCreate  ActionKind = "create"
	ActionUpdate  ActionKind = "update"
	ActionReplace ActionKind = "replace"
	ActionDelete  ActionKind = "delete"
	ActionNoop    ActionKind = "noop"
)

// Action is one changeset entry — the spec's own
// "actions[] { addon, target, env, tier, kind, kind_class, name,
// address, action, fields_changed[], sensitive, protected }" shape.
// impact, rollback and summary render (phases 23, 8) consume only this
// shape; phase 20 is the first to construct one for real. Fields are
// declared in JSON-key alphabetical order (target.go's own documented
// byte-stability convention): action, address, addon, env,
// fields_changed, kind, kind_class, name, protected, sensitive,
// target, tier.
type Action struct {
	Action        ActionKind `json:"action"`
	Address       string     `json:"address"`
	Addon         string     `json:"addon"`
	Env           string     `json:"env"`
	FieldsChanged []string   `json:"fields_changed"`
	Kind          string     `json:"kind"`
	KindClass     KindClass  `json:"kind_class"`
	Name          string     `json:"name"`
	Protected     bool       `json:"protected"`
	Sensitive     bool       `json:"sensitive"`
	Target        string     `json:"target"`
	Tier          string     `json:"tier"`
}

// Changeset is Addon.DryRun's normalized result.
type Changeset struct {
	Actions []Action `json:"actions"`
}
