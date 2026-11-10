package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// shared are the internal packages of shared machinery, not features: ADR-0006's tiers
// are a feature's. Every other package under internal is held to them.
var shared = []string{"agent", "app", "apptest", "clock", "db", "domain", "httpx", "telemetry", "web"}

// sqlCalls are the pgx methods that reach the database. Each takes a context first, so
// a call without arguments, such as r.URL.Query(), is not one.
var sqlCalls = []string{"Exec", "Query", "QueryRow", "SendBatch", "CopyFrom"}

// Each file of a feature holds one of ADR-0006's tiers, named by its prefix: web_ files
// are the only ones importing internal/web or internal/httpx, and store files the only
// ones calling the database.
func TestFeatureFilesKeepToTheirTier(t *testing.T) {
	packages, err := os.ReadDir("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range packages {
		if !p.IsDir() || slices.Contains(shared, p.Name()) {
			continue
		}
		files, err := filepath.Glob(filepath.Join("..", p.Name(), "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if !strings.HasSuffix(path, "_test.go") {
				checkTier(t, path)
			}
		}
	}
}

// checkTier reports where the file at path breaks its tier's rules.
func checkTier(t *testing.T, path string) {
	name := filepath.Base(path)
	tier := ""
	for _, prefix := range []string{"web_", "service", "store"} {
		if strings.HasPrefix(name, prefix) {
			tier = prefix
		}
	}
	if tier == "" {
		t.Errorf("%s: a feature's file is named by its tier: web_, service or store (ADR-0006)", path)
		return
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "web_" {
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); strings.HasSuffix(p, "/internal/web") || strings.HasSuffix(p, "/internal/httpx") {
				t.Errorf("%s: imports %s, which only a web_ file does (ADR-0006)", fset.Position(imp.Pos()), p)
			}
		}
	}
	if tier != "store" {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && slices.Contains(sqlCalls, sel.Sel.Name) {
				t.Errorf("%s: calls %s, which only a store file does (ADR-0006)", fset.Position(call.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}
