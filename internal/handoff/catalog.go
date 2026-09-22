package handoff

import "slices"

// CommandSpec is one entry of Catalog: enough shape for skills lint
// (phase 10) to check a skill's quoted `ai-flow …` invocation against
// the real command, ticket-key-first argument order included.
type CommandSpec struct {
	// Name is the command's nouns and verb, exactly as invoked,
	// without the "ai-flow" prefix and without the ticket key —
	// e.g. "stage start", "config init".
	Name string
	// TicketScoped is true when the ticket key is the command's first
	// positional argument.
	TicketScoped bool
	// Positional names every positional argument after the ticket key
	// (or from the start, for a command that is not ticket-scoped), in
	// order.
	Positional []string
	// Flags names every recognized flag, without its leading "--".
	Flags []string
}

// catalog is Catalog's backing data: every command that exists at the
// end of phase 3. Every later phase that adds a command appends its
// own entry here in the same PR (see the growing-surfaces registry).
var catalog = []CommandSpec{
	{Name: "version", TicketScoped: false},
	{Name: "schema", TicketScoped: false, Positional: []string{"artifact"}},
	{Name: "config init", TicketScoped: false},
	{Name: "config validate", TicketScoped: false},
	{Name: "status", TicketScoped: true},
	{Name: "stage should-run", TicketScoped: true, Positional: []string{"stage"}},
	{Name: "stage start", TicketScoped: true, Positional: []string{"stage"}},
	{Name: "stage complete", TicketScoped: true, Positional: []string{"stage"}, Flags: []string{"fail"}},
	{Name: "stage approve", TicketScoped: true, Positional: []string{"stage"}},
	{Name: "stage skip", TicketScoped: true, Positional: []string{"stage"}, Flags: []string{"reason"}},
	{Name: "stage reset", TicketScoped: true, Positional: []string{"stage"}},
	{Name: "ticket fetch", TicketScoped: true},
	{Name: "ticket update", TicketScoped: true, Flags: []string{"status", "comment", "marker"}},
	{Name: "ticket show", TicketScoped: true},
	{Name: "repo locate", TicketScoped: true},
	{Name: "repo confirm", TicketScoped: true, Flags: []string{"repos"}},
	{Name: "repo scope", TicketScoped: true},
	{Name: "git prepare", TicketScoped: true},
	{Name: "git checkout-base", TicketScoped: true},
	{Name: "git checkout-branch", TicketScoped: true},
	{Name: "git diff", TicketScoped: true, Flags: []string{"repo"}},
	{Name: "git commit", TicketScoped: true},
	{Name: "git push", TicketScoped: true},
	{Name: "pr create", TicketScoped: true},
}

// Catalog returns every command's spec. The caller's to keep — always
// a fresh copy.
func Catalog() []CommandSpec {
	return slices.Clone(catalog)
}
