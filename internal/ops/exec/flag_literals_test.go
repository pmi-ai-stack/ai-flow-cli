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
	"testing"
)

// unresolvedToolSites names every function that runs a tool whose name
// the scan cannot read from the source: neither a string literal nor a
// parameter its callers fill with one. Each lists the tools it can run,
// and its flag literals are checked against those tools' rows. A new
// such function fails TestCallSiteFlagLiteralsAreDeclared until it is
// added here, and an entry that matches no function fails too, so the
// list cannot go stale. verbs are the verb words the argv carries in
// from elsewhere (the RunnerMatch's Args), which select the rows a flag
// is looked up on.
var unresolvedToolSites = map[string]struct {
	tools []string
	verbs []string
	why   string
}{
	"internal/verify/unit/unit.go:Run": {
		tools: []string{"go", "npx", "pytest"},
		verbs: []string{"test", "jest"},
		why:   "runs detect.RunnerMatch.Command; detect's RunnerMatch literals are checked as call sites",
	},
	"internal/verify/e2e/e2e.go:Run": {
		tools: []string{"npx"},
		verbs: []string{"playwright"},
		why:   "runs detect.RunnerMatch.Command for the e2e suite (playwright through npx)",
	},
	"internal/ops/harness/harness.go:Run": {
		tools: []string{"kuttl", "chainsaw"},
		why:   "passes in.Tool to runTextScanTool for the two text-scan harness tools",
	},
}

// TestCallSiteFlagLiteralsAreDeclared closes the gap a measured corpus
// cannot: a call-site variant no unit test exercises (phase 54 found
// `helm lint <chart> -f values.yaml` that way, only through the real-tool
// suite). For every function in non-test code that runs a tool, every
// flag-shaped string literal in that function must be declared — in
// Flags or RequiredFlags — on a row for that tool whose verb's first
// word also appears as a literal there.
//
// A function runs a tool when it calls .Run(ctx, "<tool>", …) itself,
// or passes "<tool>" to a forwarder: a function that hands one of its
// own parameters to .Run as the tool (runToolJSON, applyFixOrReport,
// runTextScanTool), found to a fixpoint. A composite literal with a
// string-literal Command field (detect.RunnerMatch) is a call site of
// its own, checked against the literals inside it. Anything else is
// registered in unresolvedToolSites.
func TestCallSiteFlagLiteralsAreDeclared(t *testing.T) {
	funcs := parseModuleFuncs(t)
	fwd := findForwarders(t, funcs)

	checked, seenUnresolved := 0, map[string]bool{}
	for _, f := range funcs {
		if fwd[f.fn.Name.Name] != nil {
			// A forwarder's flags belong to its callers, which pass
			// them in; one written here would be checked against no tool.
			for _, flag := range f.lits {
				if isFlagLiteral(flag) {
					t.Errorf("%s: forwarder %s holds flag literal %q; build argv in its callers, whose tool the scan knows", f.rel, f.fn.Name.Name, flag)
				}
			}
			continue
		}

		tools, unresolved := toolsRun(f, fwd)
		lits := f.lits
		key := f.rel + ":" + f.fn.Name.Name
		if unresolved {
			site, ok := unresolvedToolSites[key]
			if !ok {
				t.Errorf("%s runs a tool named by neither a literal nor a forwarded parameter; add it to unresolvedToolSites with the tools it can run", key)
				continue
			}
			seenUnresolved[key] = true
			tools = append(tools, site.tools...)
			lits = append(slices.Clone(lits), site.verbs...)
		}
		if len(tools) == 0 {
			continue
		}
		checked++
		for _, flag := range f.lits {
			if isFlagLiteral(flag) && !declaredForCall(tools, lits, flag) {
				t.Errorf("%s: %s passes %q to %v, but no matching row declares it", f.rel, f.fn.Name.Name, flag, tools)
			}
		}
	}
	for key := range unresolvedToolSites {
		if !seenUnresolved[key] {
			t.Errorf("unresolvedToolSites[%q] matches no function that runs an unresolved tool; remove it", key)
		}
	}
	if checked < 30 {
		t.Fatalf("only %d functions running a tool found; the scan no longer sees the call sites", checked)
	}

	commands := 0
	for _, f := range funcs {
		for _, c := range f.commands {
			commands++
			for _, flag := range c.lits {
				if isFlagLiteral(flag) && !declaredForCall([]string{c.tool}, c.lits, flag) {
					t.Errorf("%s: %s builds a %s command with %q, but no matching row declares it", f.rel, f.fn.Name.Name, c.tool, flag)
				}
			}
		}
	}
	if commands < 4 {
		t.Fatalf("only %d Command literals found; the scan no longer sees detect's runner matches", commands)
	}
}

// moduleFunc is one function declaration in non-test code, with what
// the checks need from its body.
type moduleFunc struct {
	rel      string
	fn       *ast.FuncDecl
	params   map[string]int // parameter name → position
	lits     []string       // every string literal in the body
	commands []commandLit   // composite literals with a literal Command field
}

// commandLit is a composite literal naming a tool in its Command field
// (detect.RunnerMatch), with the string literals inside it.
type commandLit struct {
	tool string
	lits []string
}

func parseModuleFuncs(t *testing.T) []moduleFunc {
	t.Helper()
	root := moduleRoot(t)
	fset := token.NewFileSet()
	var out []moduleFunc
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || isExecerDecorator(fn) {
				continue
			}
			out = append(out, moduleFunc{
				rel: filepath.ToSlash(rel), fn: fn, params: paramIndex(fn),
				lits: stringLits(fn.Body), commands: commandLits(fn.Body),
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// isExecerDecorator reports whether fn is a Run method: an Execer
// implementation (missingToolRecorder in internal/cli) that passes its
// own tool parameter on. It is the interface itself, not a call site.
func isExecerDecorator(fn *ast.FuncDecl) bool {
	return fn.Recv != nil && fn.Name.Name == "Run"
}

func paramIndex(fn *ast.FuncDecl) map[string]int {
	idx := map[string]int{}
	i := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			i++
			continue
		}
		for _, n := range field.Names {
			idx[n.Name] = i
			i++
		}
	}
	return idx
}

func stringLits(n ast.Node) []string {
	var lits []string
	ast.Inspect(n, func(n ast.Node) bool {
		if s, ok := stringLit(n); ok {
			lits = append(lits, s)
		}
		return true
	})
	return lits
}

func stringLit(n ast.Node) (string, bool) {
	lit, ok := n.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

func commandLits(body *ast.BlockStmt) []commandLit {
	var out []commandLit
	ast.Inspect(body, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Command" {
				if tool, ok := stringLit(kv.Value); ok {
					out = append(out, commandLit{tool: tool, lits: stringLits(cl)})
				}
			}
		}
		return true
	})
	return out
}

// toolCall is one place a function runs a tool: an exec call
// (.Run(ctx, tool, args, opts)) or a call to a forwarder, with the
// expression in the tool position.
type toolCall struct{ tool ast.Expr }

// toolCalls returns every tool-running call in f, given the forwarders
// known so far (function name → tool parameter position).
func toolCalls(f moduleFunc, fwd map[string]*int) []toolCall {
	var calls []toolCall
	ast.Inspect(f.fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isExecRun(call, f) {
			calls = append(calls, toolCall{tool: call.Args[1]})
			return true
		}
		if i := fwd[calleeName(call)]; i != nil && *i < len(call.Args) {
			calls = append(calls, toolCall{tool: call.Args[*i]})
		}
		return true
	})
	return calls
}

// isExecRun reports whether call is Execer.Run(ctx, tool, args, opts):
// a .Run with four arguments whose tool is a string literal (the shape
// this test always matched), or whose last argument is a RunOpts — a
// composite literal or a variable declared as one. The second form
// keeps unrelated Run methods (a CheckRunner's, a harness's) out.
func isExecRun(call *ast.CallExpr, f moduleFunc) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" || len(call.Args) < 2 {
		return false
	}
	if _, ok := stringLit(call.Args[1]); ok {
		return true
	}
	return len(call.Args) == 4 && isRunOpts(call.Args[3], f.fn)
}

func isRunOpts(e ast.Expr, fn *ast.FuncDecl) bool {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return isRunOptsType(v.Type)
	case *ast.Ident:
		return identIsRunOpts(v.Name, fn)
	}
	return false
}

func isRunOptsType(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name == "RunOpts"
	case *ast.SelectorExpr:
		return t.Sel.Name == "RunOpts"
	}
	return false
}

// identIsRunOpts reports whether name is declared in fn as a RunOpts:
// a parameter of that type, `name := …RunOpts{…}` or `var name …RunOpts`.
func identIsRunOpts(name string, fn *ast.FuncDecl) bool {
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			if n.Name == name && isRunOptsType(field.Type) {
				return true
			}
		}
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name && i < len(v.Rhs) {
					if cl, ok := v.Rhs[i].(*ast.CompositeLit); ok && isRunOptsType(cl.Type) {
						found = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range v.Names {
				if id.Name == name && v.Type != nil && isRunOptsType(v.Type) {
					found = true
				}
			}
		}
		return true
	})
	return found
}

func calleeName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// findForwarders returns every function that hands one of its own
// parameters to a tool-running call as the tool, by name, with that
// parameter's position — to a fixpoint, so a forwarder of a forwarder
// counts. Two forwarders sharing a name would make calls ambiguous, so
// that fails the test.
func findForwarders(t *testing.T, funcs []moduleFunc) map[string]*int {
	t.Helper()
	fwd := map[string]*int{}
	owner := map[string]string{}
	for changed := true; changed; {
		changed = false
		for _, f := range funcs {
			name := f.fn.Name.Name
			if fwd[name] != nil {
				continue
			}
			for _, c := range toolCalls(f, fwd) {
				id, ok := c.tool.(*ast.Ident)
				if !ok {
					continue
				}
				if i, ok := f.params[id.Name]; ok {
					if prev, dup := owner[name]; dup && prev != f.rel {
						t.Fatalf("forwarder name %s is declared in both %s and %s; the scan matches calls by name", name, prev, f.rel)
					}
					fwd[name], owner[name] = &i, f.rel
					changed = true
					break
				}
			}
		}
	}
	if len(fwd) < 3 {
		t.Fatalf("only %d forwarders found (%v); the scan no longer sees runToolJSON, applyFixOrReport and runTextScanTool", len(fwd), owner)
	}
	return fwd
}

// toolsRun returns the literal tool names f runs, and whether it also
// runs one the scan cannot name.
func toolsRun(f moduleFunc, fwd map[string]*int) (tools []string, unresolved bool) {
	for _, c := range toolCalls(f, fwd) {
		if s, ok := stringLit(c.tool); ok {
			tools = append(tools, s)
			continue
		}
		unresolved = true
	}
	return tools, unresolved
}

func isFlagLiteral(s string) bool {
	return len(s) >= 2 && s[0] == '-' && s != "--" && s != "--end-of-options"
}

// declaredForCall reports whether flag is declared on a row of one of
// tools whose verb is empty or whose first verb word appears in lits.
func declaredForCall(tools, lits []string, flag string) bool {
	for _, r := range Table {
		if !slices.Contains(tools, r.Tool) {
			continue
		}
		if len(r.Verb) > 0 && !slices.Contains(lits, r.Verb[0]) {
			continue
		}
		for _, f := range r.Flags {
			if names(flag, f) || names(f, flag) {
				return true
			}
		}
		for _, f := range r.RequiredFlags {
			name, _, _ := strings.Cut(f, "=")
			if names(flag, name) || names(f, flag) {
				return true
			}
		}
	}
	return false
}
