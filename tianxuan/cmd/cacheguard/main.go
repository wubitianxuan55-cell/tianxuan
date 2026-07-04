// Command cacheguard checks the tianxuan codebase for patterns that would break
// the DeepSeek prefix cache. Reads Go source directly — zero external deps.
//
// Usage: go run ./cmd/cacheguard   or   make lint-cache
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// bootstrapDirs lists package directories where registry / runtime mutations
// are expected (startup, not mid-session).
var bootstrapDirs = []string{
	"boot", "config", "cmd", "internal/cli", "internal/serve", "desktop",
}

func isBootstrap(path string, pkg string) bool {
	if pkg == "boot" || pkg == "config" || pkg == "main" {
		return true
	}
	slashed := filepath.ToSlash(path)
	for _, d := range bootstrapDirs {
		if strings.Contains(slashed, "/"+d+"/") {
			return true
		}
	}
	return false
}

func main() {
	cwd, _ := os.Getwd()
	var issues int
	filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && (d.Name() == "vendor" || d.Name() == ".git" ||
				strings.Contains(path, "node_modules") || strings.Contains(path, "cmd/cacheguard")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		boot := isBootstrap(path, f.Name.Name)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					checkCall(fset, path, boot, n, &issues)
				case *ast.AssignStmt:
					checkAssign(fset, path, n, &issues)
				}
				return true
			})
		}
		return nil
	})
	if issues > 0 {
		fmt.Printf("\n%d cache-safety issues found.\n", issues)
		os.Exit(1)
	}
	fmt.Println("cacheguard: no cache-safety issues detected.")
}

func checkCall(fset *token.FileSet, path string, isBootstrap bool, call *ast.CallExpr, issues *int) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}
	name := sel.Sel.Name
	if !isBootstrap && x.Name == "reg" && (name == "Add" || name == "Remove") {
		pos := fset.Position(call.Pos())
		fmt.Printf("%s:%d: tool registry %s outside bootstrap — breaks prefix cache (L3)\n", path, pos.Line, name)
		*issues++
	}
	l2Methods := map[string]bool{"SetProject": true, "SetCompactL2": true, "SetLanguage": true}
	if !isBootstrap && l2Methods[name] {
		pos := fset.Position(call.Pos())
		fmt.Printf("%s:%d: L2 runtime %s outside bootstrap — prefix cache (L2) must be stable\n", path, pos.Line, name)
		*issues++
	}
}

func checkAssign(fset *token.FileSet, path string, assign *ast.AssignStmt, issues *int) {
	for _, lhs := range assign.Lhs {
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Content" {
			continue
		}
		s := exprStr(sel.X)
		if strings.Contains(s, "Messages") && strings.Contains(s, "[0]") {
			pos := fset.Position(assign.Pos())
			fmt.Printf("%s:%d: system prompt mutation — Messages[0].Content must be immutable (L1 cache)\n", path, pos.Line)
			*issues++
		}
	}
}

func exprStr(e ast.Expr) string {
	switch n := e.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return exprStr(n.X) + "." + n.Sel.Name
	case *ast.IndexExpr:
		return exprStr(n.X) + "[...]"
	default:
		return ""
	}
}
