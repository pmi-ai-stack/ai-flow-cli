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

import (
	"slices"
	"strings"
)

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
	// "rollout history". Free arguments after the verb are not matched
	// by value, but every flag-shaped one must be declared (Flags).
	Verb []string
	// RequiredFlags must all be present in argv for the row to match.
	// A bare one ("-out") is present in any spelling ("-out=x.tfplan",
	// either dash count); a "name=value" one must appear exactly and
	// not be contradicted by another spelling of the same flag (see
	// overridden).
	RequiredFlags []string
	// Flags is every flag this row's callers pass, beyond
	// RequiredFlags, as the tool spells it. Every flag-shaped argument
	// after the verb — up to "--" or "--end-of-options", after which
	// arguments are positional — must name one of them or a required
	// flag, in any spelling; anything else is refused, so a flag
	// injected through a free argument (a config value, a file the
	// agent wrote) never runs. Never nil: a row whose callers pass no
	// flags declares []string{} (TestEveryRowDeclaresItsFlags).
	Flags []string
	// ValueFlags is the subset of Flags whose next argument is their
	// value and is not checked as a flag — only where that value is
	// content that may start with "-" (git commit -m). Any other value
	// starting with "-" is refused: fail-safe, never run.
	ValueFlags []string
	// ForbiddenFlags must all be absent from argv for the row to match,
	// in any spelling — "flag", "flag=value", either dash count (see
	// names).
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
	for _, f := range r.RequiredFlags {
		if !requiredPresent(argv, f) || overridden(argv, f) {
			return false
		}
	}
	if !flagsDeclared(argv[1+len(r.Verb):], r) {
		return false
	}
	for _, f := range r.ForbiddenFlags {
		for _, a := range argv {
			if names(a, f) {
				return false
			}
		}
	}
	return true
}

// overridden reports whether a required "name=value" flag is
// contradicted elsewhere in argv: another argument naming the same flag
// bare ("--dry-run", whose value is the next argument) or with a
// different value ("--dry-run=none"). The tools this table drives take
// the last occurrence, so a contradiction anywhere is a bypass
// (2026-10-04 security review #5). A required flag with no "=" carries
// no value to contradict.
func overridden(argv []string, required string) bool {
	name, _, ok := strings.Cut(flagBody(required), "=")
	if !ok {
		return false
	}
	want := flagBody(required)
	for _, a := range argv {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		body := flagBody(a)
		if body == want {
			continue
		}
		if body == name || strings.HasPrefix(body, name+"=") {
			return true
		}
	}
	return false
}

// requiredPresent reports whether argv carries the required flag f: a
// "name=value" one exactly, a bare one in any spelling (names) — so
// "-out" is satisfied by terraform's own "-out=plan.tfplan".
func requiredPresent(argv []string, f string) bool {
	if strings.Contains(f, "=") {
		return slices.Contains(argv, f)
	}
	for _, a := range argv {
		if names(a, f) {
			return true
		}
	}
	return false
}

// flagsDeclared reports whether every flag-shaped argument in free —
// the arguments after the verb — names a declared or required flag.
// The scan stops at "--" and "--end-of-options", and skips the value
// that follows a ValueFlags flag given without "=value".
func flagsDeclared(free []string, r Row) bool {
	declared := func(arg string) bool {
		for _, f := range r.Flags {
			if names(arg, f) {
				return true
			}
		}
		for _, f := range r.RequiredFlags {
			name, _, _ := strings.Cut(f, "=")
			if names(arg, name) {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(free); i++ {
		a := free[i]
		if a == "--" || a == "--end-of-options" {
			return true
		}
		if len(a) < 2 || a[0] != '-' {
			continue
		}
		if !declared(a) {
			return false
		}
		if !strings.Contains(a, "=") && slices.ContainsFunc(r.ValueFlags, func(v string) bool { return flagBody(v) == flagBody(a) }) {
			i++ // the value, whatever it looks like
		}
	}
	return true
}

// names reports whether arg spells flag: exactly, or as "flag=value",
// with either dash count (Go's flag package, which terraform uses,
// accepts "-x" and "--x" alike). "--fix-dry-run" does not name "--fix".
func names(arg, flag string) bool {
	if !strings.HasPrefix(arg, "-") {
		return false
	}
	body, want := flagBody(arg), flagBody(flag)
	return body == want || strings.HasPrefix(body, want+"=")
}

// flagBody strips a flag's leading dashes.
func flagBody(s string) string {
	return strings.TrimLeft(s, "-")
}
