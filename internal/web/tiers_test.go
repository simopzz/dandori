package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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

// sqlWrite is a word of SQL that writes or locks rows: FOR UPDATE ends in UPDATE.
var sqlWrite = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE)\b`)

// tier is one of ADR-0006's tiers, as the prefix naming a feature's file.
type tier string

const (
	tierWeb     tier = "web_"
	tierService tier = "service"
	tierStore   tier = "store"
)

// fileTier is the tier a file's `name` puts it in: web_*, service.go or service_*,
// store.go or store_*. `ok` is false for any other `name`.
func fileTier(name string) (t tier, ok bool) {
	if strings.HasPrefix(name, string(tierWeb)) {
		return tierWeb, true
	}
	for _, t := range []tier{tierService, tierStore} {
		if name == string(t)+".go" || strings.HasPrefix(name, string(t)+"_") {
			return t, true
		}
	}
	return "", false
}

// Each file of a feature holds one of ADR-0006's tiers, named by its prefix: web_ files
// are the only ones importing internal/web or internal/httpx, store files the only ones
// calling the database, and a store file's exported functions are reads.
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

// checkTier reports where the file at `path` breaks its tier's rules.
func checkTier(t *testing.T, path string) {
	tier, ok := fileTier(filepath.Base(path))
	if !ok {
		t.Errorf("%s: a feature's file is named by its tier: web_*, service.go, service_*, store.go or store_* (ADR-0006)", path)
		return
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	if tier != tierWeb {
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); strings.HasSuffix(p, "/internal/web") || strings.HasSuffix(p, "/internal/httpx") {
				t.Errorf("%s: imports %s, which only a web_ file does (ADR-0006)", fset.Position(imp.Pos()), p)
			}
		}
	}
	if tier == tierStore {
		checkExportsRead(t, fset, f)
	} else {
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

// checkExportsRead reports an exported function of a store file whose SQL writes or
// locks: another feature, or app's wiring, calls it, and a write it may call belongs in a
// service file, holding every rule its owner holds for it.
//
// shortcut: it reads only the string literals in the function's own body, not a helper
// it calls (HasMovements locks through lock) nor a package-level const; follow those if
// an exported write slips past.
func checkExportsRead(t *testing.T, fset *token.FileSet, f *ast.File) {
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !fn.Name.IsExported() || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if ok && lit.Kind == token.STRING {
				if w := sqlWrite.FindString(lit.Value); w != "" {
					t.Errorf("%s: %s's SQL says %s, but an exported store function is a read: a write another feature may call lives in a service file, beside the rules it must hold (ADR-0006)", fset.Position(lit.Pos()), fn.Name.Name, w)
				}
			}
			return true
		})
	}
}
