// Package exec is the only package in this module allowed to import
// os/exec (see TestNoOsExecOutsideExecPackage). Every subprocess the
// product runs — addon tools, check tools, test and mutation runners,
// git — goes through the allowlist defined here; anything not on it is
// refused with error.code "exec_not_allowed".
//
// Phase 1 lands the type and an empty table: no row can ever be added
// without satisfying TestAllowlistNeverMutatesPersistentState, which is
// wired from this phase on, vacuously, against zero rows.
package exec

import "slices"

// Class is a row's blast-radius classification.
type Class string

const (
	// ClassNone touches only the worktree, a rendering or a report.
	ClassNone Class = "none"
	// ClassReadOnlyLive reads live state through a binding from
	// config.ops.environments.
	ClassReadOnlyLive Class = "read_only_live"
	// ClassRepoWrite changes a git repository the developer owns, or
	// that repository on the git host; never an infrastructure target.
	ClassRepoWrite Class = "repo_write"
	// ClassEphemeral acts on the ticket's kind cluster or a
	// sandbox-tier workspace the CLI created and will delete.
	ClassEphemeral Class = "ephemeral"
)

// Row is one allowlisted (tool, verb) pair ai-flow may execute as a
// subprocess.
type Row struct {
	// Tool is matched against argv[0].
	Tool string
	// Verb is matched against the verb position: argv[1] for a
	// one-word verb, argv[1:3] for a two-word verb such as
	// "rollout history". Free arguments after the verb are never
	// matched.
	Verb []string
	// RequiredFlags must all be present in argv for the row to match.
	RequiredFlags []string
	// ForbiddenFlags must all be absent from argv for the row to match.
	ForbiddenFlags []string
	// Mutating declares whether this verb changes state for this tool.
	// Every row with Mutating true must satisfy
	// TestAllowlistNeverMutatesPersistentState.
	Mutating bool
	// Class is this row's blast-radius classification.
	Class Class
}

// Allowlist is the enumeration of every external tool ai-flow may run.
// It is empty in phase 1; later phases add rows, one PR at a time, each
// extending the invariant tests in the same PR.
type Allowlist struct {
	rows []Row
}

// NewAllowlist builds an Allowlist from rows.
func NewAllowlist(rows ...Row) *Allowlist {
	return &Allowlist{rows: append([]Row(nil), rows...)}
}

// Rows returns a copy of the allowlist's rows, in table order.
func (a *Allowlist) Rows() []Row {
	return append([]Row(nil), a.rows...)
}

// Match finds the row whose (tool, verb) matches argv. Free arguments —
// everything after the verb position — are never matched; RequiredFlags
// and ForbiddenFlags are checked against the full argv.
func (a *Allowlist) Match(argv []string) (Row, bool) {
	if len(argv) == 0 {
		return Row{}, false
	}
	tool := argv[0]
	rest := argv[1:]
	for _, r := range a.rows {
		if r.Tool != tool {
			continue
		}
		if !verbMatches(rest, r.Verb) {
			continue
		}
		if !flagsSatisfied(argv, r) {
			continue
		}
		return r, true
	}
	return Row{}, false
}

func verbMatches(rest, verb []string) bool {
	if len(rest) < len(verb) {
		return false
	}
	return slices.Equal(rest[:len(verb)], verb)
}

func flagsSatisfied(argv []string, r Row) bool {
	present := make(map[string]bool, len(argv))
	for _, a := range argv {
		present[a] = true
	}
	for _, f := range r.RequiredFlags {
		if !present[f] {
			return false
		}
	}
	for _, f := range r.ForbiddenFlags {
		if present[f] {
			return false
		}
	}
	return true
}
