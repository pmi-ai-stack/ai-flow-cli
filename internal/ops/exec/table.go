package exec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Table is the allowlist this binary actually ships. It was empty
// through phase 3 — every later phase appends its own rows here, in
// the same PR that needs them (see
// TestAllowlistNeverMutatesPersistentState in allowlist_test.go, which
// runs against exactly this table). internal/handoff hashes it into
// active.json's allowlist_hash from phase 3 on, so the guard (phase 9)
// and doctor (phase 8) can tell an active.json written by a different
// CLI version apart from one written by this build.
//
// Phase 5 adds git's rows — the CLI's first subprocess. Every row is
// class: repo_write (the spec's own git paragraph names no class: none
// git row — see the phase-5 plan's design decision 15): the local ones
// (fetch, worktree add, checkout — reachable only from git prepare,
// checkout-base, checkout-branch) touch nothing outside the developer's
// own checkout; the read-only helpers (status, rev-parse, rev-list,
// merge-base, remote get-url, diff) never mutate anything but still
// only ever touch a git repository, so they stay on the same class
// rather than a class: none this codebase has no other use for yet;
// the publishing ones (add, commit, push — reachable only from git
// commit and git push) are gated on a recorded ship approval at the
// internal/cli layer, not by the allowlist itself. git tag is named in
// the spec's own publishing list but has no row here: no command in
// this phase's scope ever calls it, and "nothing else may ever be
// added" (this file's own rule) means no row ships without a caller.
var Table = []Row{
	{Tool: "git", Verb: []string{"status"}, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"rev-parse"}, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"rev-list"}, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"merge-base"}, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"remote", "get-url"}, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"diff"}, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"ls-files"}, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"fetch"}, Mutating: true, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"worktree", "add"}, Mutating: true, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"checkout"}, Mutating: true, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"add"}, Mutating: true, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"commit"}, Mutating: true, Class: ClassRepoWrite},
	{Tool: "git", Verb: []string{"push"}, Mutating: true, Class: ClassRepoWrite},

	// Phase 6 adds the CLI's first class: none rows — internal/verify's
	// unit and e2e runners. None of these mutates anything outside the
	// worktree (build caches, node_modules, __pycache__) or writes
	// anything but a report ClassNone's own doc comment already
	// describes, so none needed a dry-run flag, an ephemeral class or
	// the repo_write publishing gate git's own mutating rows use.
	{Tool: "go", Verb: []string{"test"}, Class: ClassNone},
	{Tool: "npx", Verb: []string{"jest"}, Class: ClassNone},
	{Tool: "pytest", Verb: []string{}, Class: ClassNone},
	{Tool: "npx", Verb: []string{"playwright", "test"}, Class: ClassNone},

	// Phase 7 adds internal/mutation's four tools — same class: none
	// reasoning as phase 6's runners: each touches only the worktree
	// (build caches, node_modules) and writes only a report.
	{Tool: "gremlins", Verb: []string{"unleash"}, Class: ClassNone},
	{Tool: "npx", Verb: []string{"stryker", "run"}, Class: ClassNone},
	{Tool: "mutmut", Verb: []string{"run"}, Class: ClassNone},
	{Tool: "mutmut", Verb: []string{"junitxml"}, Class: ClassNone},
	{Tool: "mvn", Verb: []string{"org.pitest:pitest-maven:mutationCoverage"}, Class: ClassNone},
}

// Hash returns a stable hex-encoded SHA-256 digest of Table's current
// contents. It changes exactly when a row is added, removed or
// modified — never on call order, since Table is a plain slice marshalled
// as a JSON array (order is already the table's own declared order, not
// incidental map iteration).
func Hash() string {
	data, err := json.Marshal(Table)
	if err != nil {
		// Row is a plain struct of strings, slices of strings and bools —
		// it cannot fail to marshal. A failure here is a programmer error
		// (04-errors.md rule 9's must* pattern), not an input this
		// function's caller can act on.
		panic(fmt.Sprintf("exec: marshal Table: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
