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

// TestCallSiteFlagLiteralsAreDeclared closes the gap a measured corpus
// cannot: a call-site variant no unit test exercises (phase 54 found
// `helm lint <chart> -f values.yaml` that way, only through the real-tool
// suite). For every function in non-test code that calls
// .Run(ctx, "<tool>", …), every flag-shaped string literal in that
// function must be declared — in Flags or RequiredFlags — on a row for
// that tool whose verb's first word also appears as a literal there.
func TestCallSiteFlagLiteralsAreDeclared(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	checked := 0
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
			if !ok || fn.Body == nil {
				continue
			}
			tools, lits := runToolsAndLiterals(fn)
			if len(tools) == 0 {
				continue
			}
			checked++
			for _, flag := range lits {
				if len(flag) < 2 || flag[0] != '-' || flag == "--" || flag == "--end-of-options" {
					continue
				}
				if !declaredForCall(tools, lits, flag) {
					t.Errorf("%s: %s passes %q to %v, but no matching row declares it", rel, fn.Name.Name, flag, tools)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 30 {
		t.Fatalf("only %d functions with a literal .Run tool found; the scan no longer sees the call sites", checked)
	}
}

// runToolsAndLiterals returns the literal tool names fn passes as the
// second argument of a .Run call, and every string literal in fn.
func runToolsAndLiterals(fn *ast.FuncDecl) (tools, lits []string) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				if s, err := strconv.Unquote(v.Value); err == nil {
					lits = append(lits, s)
				}
			}
		case *ast.CallExpr:
			sel, ok := v.Fun.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "Run" && len(v.Args) >= 2 {
				if lit, ok := v.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						tools = append(tools, s)
					}
				}
			}
		}
		return true
	})
	return tools, lits
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
