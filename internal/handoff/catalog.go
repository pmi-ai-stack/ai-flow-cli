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
	// SkipToolRunEvent is true for a command internal/cli/root.go's wrap
	// must not log its generic ToolRunEvent for: the five state-changing
	// `stage *` commands already append their own, strictly richer
	// StageTransitionEvent for the identical call, and `stage
	// should-run` changes nothing at all (a read-only preview) — either
	// way, a second, less specific events.jsonl line would be redundant
	// or wrong, not merely duplicate.
	SkipToolRunEvent bool
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
	{Name: "stage should-run", TicketScoped: true, Positional: []string{"stage"}, SkipToolRunEvent: true},
	{Name: "stage start", TicketScoped: true, Positional: []string{"stage"}, SkipToolRunEvent: true},
	{Name: "stage complete", TicketScoped: true, Positional: []string{"stage"}, Flags: []string{"fail"}, SkipToolRunEvent: true},
	{Name: "stage approve", TicketScoped: true, Positional: []string{"stage"}, SkipToolRunEvent: true},
	{Name: "stage skip", TicketScoped: true, Positional: []string{"stage"}, Flags: []string{"reason"}, SkipToolRunEvent: true},
	{Name: "stage reset", TicketScoped: true, Positional: []string{"stage"}, SkipToolRunEvent: true},
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

	// Phase 6.
	{Name: "detect languages", TicketScoped: true},
	{Name: "detect runners", TicketScoped: true},
	{Name: "detect mutation", TicketScoped: true},
	{Name: "detect e2e", TicketScoped: true},
	{Name: "detect frontend", TicketScoped: true},
	{Name: "test run", TicketScoped: true, Flags: []string{"repo"}},
	{Name: "e2e run", TicketScoped: true, Flags: []string{"case"}},
	{Name: "visual run", TicketScoped: true, Flags: []string{"screen"}},
	{Name: "visual judge", TicketScoped: true, Flags: []string{"screen", "verdict", "reason"}},

	// Phase 7.
	{Name: "mutation scope", TicketScoped: true},
	{Name: "mutation run", TicketScoped: true},
	{Name: "mutation summarize", TicketScoped: true, Flags: []string{"equivalent", "reason", "accept"}},

	// Phase 8.
	{Name: "summary render", TicketScoped: true},
	{Name: "doctor", TicketScoped: false},

	// Phase 9.
	{Name: "hook guard", TicketScoped: false},
	{Name: "hook session-start", TicketScoped: false, Flags: []string{"plugin-root"}},
	{Name: "hook selftest", TicketScoped: false, Flags: []string{"host"}},
	{Name: "hooks install", TicketScoped: false, Flags: []string{"host", "scope"}},

	// Phase 10.
	{Name: "skills lint", TicketScoped: false, Positional: []string{"dir"}},
	{Name: "skills install", TicketScoped: false, Flags: []string{"from", "into"}},

	// Phase 11.
	{Name: "skills evals", TicketScoped: false, Positional: []string{"dir"}, Flags: []string{"skill", "host", "model", "out"}},

	// Phase 12.
	{Name: "standards validate", TicketScoped: false, Positional: []string{"dir"}},
	{Name: "standards sync", TicketScoped: false, Flags: []string{"source"}},
	{Name: "standards export", TicketScoped: false, Positional: []string{"dir"}},

	// Phase 13.
	{Name: "standards resolve", TicketScoped: true},
	{Name: "standards show", TicketScoped: true, Flags: []string{"id"}},
	{Name: "standards waive", TicketScoped: true, Flags: []string{"check", "repo", "reason", "expires", "confirmed"}},

	// Phase 14. Registered ticket-scoped only — the directory (CI) form
	// is never invoked from a skill (the phase-14 plan's design
	// decision 11).
	{Name: "standards check", TicketScoped: true, Flags: []string{"fix", "stage"}},

	// Phase 18.
	{Name: "ops detect", TicketScoped: true},
	{Name: "ops addons", TicketScoped: false},
	{Name: "ops allowlist", TicketScoped: false},

	// Phase 19.
	{Name: "ops render", TicketScoped: true, Flags: []string{"ref", "target"}},

	// Phase 20.
	{Name: "ops drift", TicketScoped: true, Flags: []string{"target"}},
	{Name: "ops dry-run", TicketScoped: true, Flags: []string{"offline", "target"}},

	// Phase 23.
	{Name: "ops impact", TicketScoped: true},
	{Name: "ops rollback", TicketScoped: true},
}

// Catalog returns every command's spec. The caller's to keep — always
// a fresh copy.
func Catalog() []CommandSpec {
	return slices.Clone(catalog)
}

// IsTicketScoped reports whether name (a Catalog() command name, e.g.
// "mutation run") is ticket-scoped, per Catalog()'s own TicketScoped
// field. A name Catalog() does not list at all reports false — never
// an error — since internal/cli/root.go's wrap calls this for every
// command including ones outside this package's own knowledge (a
// caller bug there is not this function's to detect).
func IsTicketScoped(name string) bool {
	for _, c := range catalog {
		if c.Name == name {
			return c.TicketScoped
		}
	}
	return false
}

// SkipToolRunEvent reports whether name (a Catalog() command name)
// must not get wrap's generic ToolRunEvent, per Catalog()'s own
// SkipToolRunEvent field. A name Catalog() does not list at all
// reports false, the same tolerant-of-unknown-names shape
// IsTicketScoped already uses.
func SkipToolRunEvent(name string) bool {
	for _, c := range catalog {
		if c.Name == name {
			return c.SkipToolRunEvent
		}
	}
	return false
}
