package exec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestGitPublishingVerbsReachableOnlyFromGitopsPublish is the
// reachability test the phase-5 plan deferred to this phase ("holds
// today by construction" — ai-flow-cli/AGENTS.md's own phase-8
// paragraph): git's publishing verbs (add, commit, push, tag — the
// spec's own "the publishing ones") must appear as a literal call
// argument nowhere but internal/gitops/publish.go.
//
// A plain substring scan (mirroring noexec_outside_test.go's own
// style) was tried first and rejected: "commit"/"push"/"tag" appear
// as struct/map/const literals and JSON tags all over this module
// (internal/agenthost/guard/rows.go's own protected-table row,
// internal/standards's RefKind consts, internal/cli/checks.go's
// ticketOnlyKinds, internal/cli/version.go's Commit field) — none of
// them a call argument. Scoping the scan to string literals that are
// direct arguments of a go/ast CallExpr excludes every one of those
// false positives while still catching the real call site in
// publish.go (g.run(ctx, dir, "commit", "-m", message)) and any future
// misuse anywhere else in the module.
func TestGitPublishingVerbsReachableOnlyFromGitopsPublish(t *testing.T) {
	watched := map[string]bool{"add": true, "commit": true, "push": true, "tag": true}
	allowed := []string{filepath.Join("internal", "gitops", "publish.go")}

	violations := scanCallArgLiterals(t, moduleRoot(t), watched, allowed, suppressWorktreeAdd)
	for _, v := range violations {
		t.Errorf("%s: %q used as a call argument outside %v — git's publishing verbs must stay reachable only from git commit/git push", v.file, v.literal, allowed)
	}
}

// suppressWorktreeAdd is TestGitPublishingVerbsReachableOnlyFromGitopsPublish's
// own business rule, not the scanner's: "git worktree add" is its own,
// distinct two-word verb (already its own Table row) — its second
// word must not be mistaken for the single-word "git add" (staging)
// this test is actually watching for. Kept as a caller-supplied
// suppress predicate rather than baked into scanCallArgLiterals
// itself, since no other watch list needs it.
func suppressWorktreeAdd(callLiterals []string, literal string) bool {
	return literal == "add" && slices.Contains(callLiterals, "worktree")
}

// TestEphemeralRowsReachableOnlyFromDesignatedCallers is phase 18's own
// "Reachability tests: ephemeral rows are reachable only from ops test
// run and the cluster lifecycle commands" bullet. The scanner watches
// by bare tool name, not (tool, verb) — "terraform" is watched
// regardless of verb because Table carries one ClassEphemeral row for
// it ("terraform test"), so phase 21's own real, non-ephemeral calls
// ("init", "validate", "plan", "show", "workspace select") trip it
// exactly as much as a real "terraform test" call would; the phase-21
// plan's own §0.3 already anticipated this and its allowed entries
// below are exactly its own non-ephemeral call sites, added in that
// phase's own PR — not internal/ops/addons/terraform/harness.go,
// which only detects .tftest.hcl files on disk and calls no
// subprocess at all. No command exists yet for the ephemeral verb
// itself ("terraform test", "kuttl", "chainsaw", ...) — phase 24 will
// need its own, separate, explicit update to allowed the day it adds
// the first real caller of that verb.
func TestEphemeralRowsReachableOnlyFromDesignatedCallers(t *testing.T) {
	watched := map[string]bool{}
	for _, r := range Table {
		if r.Class == ClassEphemeral {
			watched[r.Tool] = true
		}
	}
	if len(watched) == 0 {
		t.Fatal("no ClassEphemeral row on Table — this test has nothing to check; if that's expected, delete it")
	}

	allowed := []string{
		filepath.Join("internal", "ops", "addons", "terraform", "render.go"),
		filepath.Join("internal", "ops", "addons", "terraform", "drift.go"),
		filepath.Join("internal", "ops", "addons", "terraform", "dryrun.go"),
		filepath.Join("internal", "ops", "addons", "terraform", "env.go"),
	}
	violations := scanCallArgLiterals(t, moduleRoot(t), watched, allowed, nil)
	for _, v := range violations {
		t.Errorf("%s: %q (an ephemeral-class tool) used as a call argument outside %v", v.file, v.literal, allowed)
	}
}

// TestScanCallArgLiteralsCatchesAViolation proves the scanner above is
// not vacuously true by construction: a call site outside the allowed
// list, passing a watched literal, is actually detected.
func TestScanCallArgLiteralsCatchesAViolation(t *testing.T) {
	dir := t.TempDir()
	const src = `package fixture

func caller() {
	run("git", "commit", "-m", "message")
}

func run(tool string, args ...string) {}
`
	if err := os.WriteFile(filepath.Join(dir, "caller.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	hits := scanCallArgLiterals(t, dir, map[string]bool{"commit": true}, nil, nil)
	if len(hits) != 1 || hits[0].literal != "commit" {
		t.Fatalf("scanCallArgLiterals = %v, want one hit for %q", hits, "commit")
	}

	// The same file, now in the allowed list, produces no hit.
	rel, err := filepath.Rel(dir, filepath.Join(dir, "caller.go"))
	if err != nil {
		t.Fatal(err)
	}
	if hits := scanCallArgLiterals(t, dir, map[string]bool{"commit": true}, []string{rel}, nil); len(hits) != 0 {
		t.Errorf("scanCallArgLiterals with the file allowed = %v, want none", hits)
	}
}

type literalHit struct {
	file    string
	literal string
}

// callLiterals is every string-literal argument found in one call
// expression, together with the file (relative to the root that was
// walked) it came from — the unit moduleCallsCache stores, and the
// unit scanCallArgLiterals filters per caller.
type callLiterals struct {
	file     string
	literals []string
}

// moduleCallsCache memoizes callsIn's walk-and-parse pass per root:
// this file's two real reachability tests both scan the same,
// unchanging module tree (moduleRoot(t)) with different watch lists,
// so parsing every non-test .go file (~190 files) a second time for
// the second test would be pure repeated work for an answer the first
// scan already computed.
var (
	moduleCallsCacheMu sync.Mutex
	moduleCallsCache   = map[string][]callLiterals{}
)

// callsIn returns every call expression's own string-literal argument
// list found while walking root (every non-test .go file, excluding
// .git and testdata), cached by root so a second scan of the same
// tree is free.
func callsIn(t *testing.T, root string) []callLiterals {
	t.Helper()
	moduleCallsCacheMu.Lock()
	defer moduleCallsCacheMu.Unlock()
	if cached, ok := moduleCallsCache[root]; ok {
		return cached
	}

	fset := token.NewFileSet()
	var calls []callLiterals
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		src, readErr := os.ReadFile(path) // G304: path comes from filepath.WalkDir over this module's own tree, not external input; gosec does not run on _test.go files anyway
		if readErr != nil {
			return readErr
		}
		file, parseErr := parser.ParseFile(fset, path, src, 0)
		if parseErr != nil {
			return parseErr
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var lits []string
			for _, arg := range call.Args {
				if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil {
						lits = append(lits, v)
					}
				}
			}
			if len(lits) > 0 {
				calls = append(calls, callLiterals{file: rel, literals: lits})
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	moduleCallsCache[root] = calls
	return calls
}

// scanCallArgLiterals returns every occurrence of a watched string
// literal used as a direct call argument somewhere under root, whose
// containing file is not one of allowed. suppress, when non-nil, is
// consulted per literal with every literal argument of its own call —
// a caller's own business rule for excluding a literal that would
// otherwise match (e.g. distinguishing a two-word verb's second word
// from an unrelated single-word one); pass nil when no exception is
// needed.
func scanCallArgLiterals(t *testing.T, root string, watched map[string]bool, allowed []string, suppress func(callLiterals []string, literal string) bool) []literalHit {
	t.Helper()
	var hits []literalHit
	for _, c := range callsIn(t, root) {
		if slices.Contains(allowed, c.file) {
			continue
		}
		for _, value := range c.literals {
			if !watched[value] {
				continue
			}
			if suppress != nil && suppress(c.literals, value) {
				continue
			}
			hits = append(hits, literalHit{file: c.file, literal: value})
		}
	}
	return hits
}
