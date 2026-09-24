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

	// Phase 14 adds internal/standards/check's eight kinds' tools — same
	// class: none reasoning throughout: every one of these reads or
	// rewrites files inside the developer's own worktree (a source file,
	// its own config, a report) and none reaches a persistent
	// infrastructure target, so none is Mutating even where it rewrites
	// files in place (gofmt -w, prettier --write, ruff format) — the
	// same bucket phase 6/7's test and mutation tools are already in,
	// not the git rows' repo_write/dry-run-flag shape (see the
	// phase-14 plan's design decision 7). format's check and fix modes
	// are split into their own rows (RequiredFlags/ForbiddenFlags) so
	// each is independently the only shape it can be invoked as.
	{Tool: "gofmt", Verb: []string{}, RequiredFlags: []string{"-l"}, Class: ClassNone},
	{Tool: "gofmt", Verb: []string{}, RequiredFlags: []string{"-w"}, Class: ClassNone},
	{Tool: "prettier", Verb: []string{}, RequiredFlags: []string{"--check"}, Class: ClassNone},
	{Tool: "prettier", Verb: []string{}, RequiredFlags: []string{"--write"}, Class: ClassNone},
	{Tool: "ruff", Verb: []string{"format"}, RequiredFlags: []string{"--check"}, Class: ClassNone},
	{Tool: "ruff", Verb: []string{"format"}, ForbiddenFlags: []string{"--check"}, Class: ClassNone},
	{Tool: "ruff", Verb: []string{"check"}, ForbiddenFlags: []string{"--fix"}, Class: ClassNone},
	{Tool: "eslint", Verb: []string{}, ForbiddenFlags: []string{"--fix"}, Class: ClassNone},
	{Tool: "golangci-lint", Verb: []string{"run"}, Class: ClassNone},
	{Tool: "checkstyle", Verb: []string{}, Class: ClassNone},
	{Tool: "gitleaks", Verb: []string{"detect"}, Class: ClassNone},

	// Phase 18 adds every OPS addon-tool row this phase's own plan can
	// commit to without inventing an unverified invocation shape — see
	// the phase-18 plan's design decision 8 and §1.2. No Addon
	// implementation exists yet (terraform/kubectl land in 21/22,
	// helm/kustomize in 26, github-actions/jenkins in 27); the
	// allowlist is data, decoupled from any Go type, so a row needs no
	// caller to exist yet, only a test (this file's own
	// TestAllowlistNeverMutatesPersistentState, extended below).
	//
	// Deliberately absent: Jenkins's declarative-linter/get-job (Jenkins
	// CLI reachability is an open question owned by phase 27, not this
	// one — inventing an invocation shape now would be a guess this
	// phase has no authority to make); kind create/delete cluster (the
	// growing-surfaces registry credits phase 24, not 18, and no addon
	// name lists "kind" at all — it is infra-test's own cluster, not a
	// capability-matrix tool); conftest/checkov/trivy config/kyverno
	// apply (policy/scan, explicitly phase 25's own line).
	{Tool: "terraform", Verb: []string{"init"}, RequiredFlags: []string{"-backend=false"}, Class: ClassNone},
	{Tool: "terraform", Verb: []string{"validate"}, Class: ClassNone},
	{Tool: "tflint", Verb: []string{}, Class: ClassNone},
	{Tool: "terraform", Verb: []string{"plan"}, RequiredFlags: []string{"-detailed-exitcode"}, Class: ClassReadOnlyLive},
	{Tool: "terraform", Verb: []string{"plan"}, RequiredFlags: []string{"-out"}, Class: ClassReadOnlyLive},
	{Tool: "terraform", Verb: []string{"show"}, RequiredFlags: []string{"-json"}, Class: ClassNone},
	// Phase 21's own two additions: Drift/DryRun both need a *real*
	// (backend-reading) init — distinct from the -backend=false row
	// above, which Render uses and which never touches live state — and
	// a workspace switch before planning. Neither is Mutating: an init
	// or workspace select touches only local .terraform/ metadata and a
	// state lock, never the target infrastructure itself (the same
	// "touches only the worktree" bucket ClassReadOnlyLive's own doc
	// comment already puts kubectl diff/helm get manifest in).
	{Tool: "terraform", Verb: []string{"init"}, ForbiddenFlags: []string{"-backend=false"}, Class: ClassReadOnlyLive},
	{Tool: "terraform", Verb: []string{"workspace", "select"}, Class: ClassReadOnlyLive},
	// Ephemeral per the Execution allowlist section's own list (line
	// 367): only ops test run/teardown may ever reach this row — see
	// reachability_test.go.
	{Tool: "terraform", Verb: []string{"test"}, Mutating: true, Class: ClassEphemeral},

	{Tool: "kubeconform", Verb: []string{}, Class: ClassNone},
	{Tool: "kubectl", Verb: []string{"diff"}, Class: ClassReadOnlyLive},
	// Phase 22's own addition: Drift/DryRun both need a live read of a
	// target's current state ("before," for the JSON comparison neither
	// diff's own external-tool-dependent text output nor the capability
	// matrix's literal cell name — see .ai/phase-22/plan.md §0.2/§0.9).
	// Non-mutating: a get touches nothing.
	{Tool: "kubectl", Verb: []string{"get"}, Class: ClassReadOnlyLive},
	// The dry-run flag exception (line 366): matched per row, never by
	// name across tools — this row's own flag is "--dry-run=server".
	{Tool: "kubectl", Verb: []string{"apply"}, RequiredFlags: []string{"--dry-run=server"}, Mutating: true, Class: ClassReadOnlyLive},
	{Tool: "kubectl", Verb: []string{"rollout", "history"}, Class: ClassReadOnlyLive},
	// Phase 20's own doctor live probe (spec line 470: "kubectl auth
	// can-i --list" → "read_only") — addon-independent, no capability
	// matrix column names it, added here for the same reason phase 18's
	// own kubectl rows above predate any kubectl addon (phase 22).
	{Tool: "kubectl", Verb: []string{"auth", "can-i"}, RequiredFlags: []string{"--list"}, Class: ClassReadOnlyLive},
	{Tool: "kuttl", Verb: []string{}, Mutating: true, Class: ClassEphemeral},
	{Tool: "chainsaw", Verb: []string{}, Mutating: true, Class: ClassEphemeral},

	{Tool: "helm", Verb: []string{"template"}, Class: ClassNone},
	{Tool: "helm", Verb: []string{"lint"}, Class: ClassNone},
	{Tool: "helm", Verb: []string{"get", "manifest"}, Class: ClassReadOnlyLive},
	// helm diff is declared non-mutating (phase 26's own line) even
	// though it reads a live release, since it never applies anything.
	{Tool: "helm", Verb: []string{"diff", "upgrade"}, Class: ClassReadOnlyLive},
	{Tool: "helm", Verb: []string{"history"}, Class: ClassReadOnlyLive},
	{Tool: "helm", Verb: []string{"unittest"}, Class: ClassNone},
	{Tool: "ct", Verb: []string{"lint"}, Class: ClassNone},
	{Tool: "ct", Verb: []string{"install"}, Mutating: true, Class: ClassEphemeral},

	{Tool: "kustomize", Verb: []string{"build"}, Class: ClassNone},

	{Tool: "actionlint", Verb: []string{}, Class: ClassNone},
	// act's own dry-run flag is "-n" (line 366); a free "-j <job>"
	// alongside it still matches this row first (declared before the
	// ephemeral row below), so "act -j JOB -n" stays the dry job-graph
	// invocation, not the real one.
	{Tool: "act", Verb: []string{}, RequiredFlags: []string{"-n"}, Mutating: true, Class: ClassNone},
	// The real, opt-in job execution (line 367's own "act -j"): only
	// reachable without "-n".
	{Tool: "act", Verb: []string{}, RequiredFlags: []string{"-j"}, ForbiddenFlags: []string{"-n"}, Mutating: true, Class: ClassEphemeral},

	{Tool: "jenkinsfile-runner", Verb: []string{}, Mutating: true, Class: ClassEphemeral},
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
