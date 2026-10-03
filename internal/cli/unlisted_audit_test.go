package cli

import (
	"bufio"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// unlistedRunCommands is every unlisted_run event's command in path,
// in order.
func unlistedRunCommands(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e handoff.Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("events line %q: %v", sc.Text(), err)
		}
		if e.Type == handoff.EventTypeUnlistedRun {
			out = append(out, e.Command)
		}
	}
	return out
}

// TestVisualRunRecordsEveryUnlistedProcess is SEC-4: both processes
// visual run starts outside the allowlist — the dev server and the
// capture script, the one the spec's "exactly two" sentence did not
// name — leave a line in the ticket's events.jsonl.
func TestVisualRunRecordsEveryUnlistedProcess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	home := t.TempDir()
	if _, _, ec := runRoot(t, "config", "init", "--home", home); ec != 0 {
		t.Fatalf("config init: exitCode = %d", ec)
	}
	repoDir := t.TempDir()
	writeVisualConfig(t, repoDir, srv.URL)
	writeRepoJSONFixture(t, home, "PROJ-1", []fixtureRepo{{"web", repoDir}}, nil)
	useFakeUnlistedRunner(t)

	if stdout, _, ec := runRoot(t, "visual", "run", "PROJ-1", "--screen", "/a", "--screen", "/b", "--home", home); ec != 0 {
		t.Fatalf("visual run: exitCode = %d\n%s", ec, stdout.String())
	}

	got := unlistedRunCommands(t, handoff.TicketEventsPath(home, "PROJ-1"))
	if len(got) != 3 || got[0] != "npm run dev" || !strings.HasPrefix(got[1], "node capture.js") || !strings.HasPrefix(got[2], "node capture.js") {
		t.Errorf("unlisted_run events = %q, want the dev server then one capture per screen", got)
	}
}

// TestSkillsEvalsRecordsHostRuns: every headless claude invocation —
// the host run and the grader, per scenario — is recorded in the
// global events.jsonl (no ticket owns an eval run).
func TestSkillsEvalsRecordsHostRuns(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	home := t.TempDir()
	dir := t.TempDir()
	buildEvalsFixtureSkill(t, dir, "hello", "PROJ-1")
	useSequencedEvalRunner(t, passingRunAndGrade()...)

	if stdout, _, ec := runRoot(t, "skills", "evals", dir, "--home", home, "--out", t.TempDir()); ec != 0 {
		t.Fatalf("skills evals: exitCode = %d\n%s", ec, stdout.String())
	}
	got := unlistedRunCommands(t, handoff.GlobalEventsPath(home))
	if len(got) != 6 {
		t.Fatalf("unlisted_run events = %d (%q), want 6: 3 scenarios × (run + grade)", len(got), got)
	}
	for _, c := range got {
		if !strings.HasPrefix(c, "claude ") {
			t.Errorf("event command %q, want a claude invocation", c)
		}
	}
}

// TestNewUnlistedRunnerOnlyCalledThroughAudit makes the audit
// structural: outside tests, newUnlistedRunner() is called exactly
// once, inside unlistedRunnerFor — so a future command cannot start an
// unlisted process without its events.jsonl line.
func TestNewUnlistedRunnerOnlyCalledThroughAudit(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var callers []string
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "newUnlistedRunner" {
						callers = append(callers, fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	if !slices.Equal(callers, []string{"unlistedRunnerFor"}) {
		t.Errorf("newUnlistedRunner() called from %v, want only unlistedRunnerFor", callers)
	}
}
