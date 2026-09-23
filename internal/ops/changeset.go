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
// shape; phase 20 is the first to construct one for real.
type Action struct {
	Addon         string
	Target        string
	Env           string
	Tier          string
	Kind          string
	KindClass     KindClass
	Name          string
	Address       string
	Action        ActionKind
	FieldsChanged []string
	Sensitive     bool
	Protected     bool
}

// Changeset is Addon.DryRun's normalized result.
type Changeset struct {
	Actions []Action
}
