package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Every string a page shows has both translations: each Message gives En and It, and a
// format fills the same verbs in both.
func TestEveryMessageHasBothTranslations(t *testing.T) {
	msgs := messages(t)
	if len(msgs) == 0 {
		t.Fatal("found no Message literals: has Message moved?")
	}
	for _, m := range msgs {
		if m.err != "" {
			t.Errorf("%s: %s", m.pos, m.err)
			continue
		}
		if m.en == "" || m.it == "" {
			t.Errorf("%s: Message %q / %q lacks a translation", m.pos, m.en, m.it)
		}
		if en, it := verbs.FindAllString(m.en, -1), verbs.FindAllString(m.it, -1); strings.Join(en, " ") != strings.Join(it, " ") {
			t.Errorf("%s: Message %q fills %v, but its Italian %q fills %v", m.pos, m.en, en, m.it, it)
		}
	}
}

var verbs = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

// Pages show no text of their own: what a template shows is a Message, so it has both
// translations. The name Dandori, Japanese and punctuation are the same in every
// Language. String literals passed to components are not checked: a label goes in as a
// Message too.
func TestTemplatesShowOnlyMessages(t *testing.T) {
	for path, src := range ownFrontendSources(t) {
		if filepath.Ext(path) != ".templ" {
			continue
		}
		for _, line := range visibleText(src) {
			if latin.MatchString(strings.ReplaceAll(line.text, "Dandori", "")) {
				t.Errorf("%s line %d shows text of its own, not a Message: %s", path, line.line, line.text)
			}
		}
		for i, line := range strings.Split(src, "\n") {
			if shownAttribute.MatchString(line) {
				t.Errorf("%s line %d shows an attribute of its own, not a Message: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}

var (
	latin          = regexp.MustCompile(`[A-Za-z]`)
	shownAttribute = regexp.MustCompile(`\b(aria-label|title|placeholder|alt)="[^"]*[A-Za-z]`)
)

// foundMessage is a Message literal in the source.
type foundMessage struct {
	pos    string
	en, it string
	// err says why its texts couldn't be read.
	err string
}

// messages are every Message literal in our Go source: web.Message{…}, and the elements
// of an array, slice or map of them.
func messages(t *testing.T) []foundMessage {
	t.Helper()
	var found []foundMessage
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_templ.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		inWeb := file.Name.Name == "web"
		message := func(typ ast.Expr) bool { return isMessage(typ, inWeb) }
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			switch typ := lit.Type.(type) {
			case *ast.ArrayType:
				if message(typ.Elt) {
					for _, e := range lit.Elts {
						found = append(found, readMessage(fset, e))
					}
					return false
				}
			case *ast.MapType:
				if message(typ.Value) {
					for _, e := range lit.Elts {
						if kv, ok := e.(*ast.KeyValueExpr); ok {
							found = append(found, readMessage(fset, kv.Value))
						}
					}
					return false
				}
			default:
				if message(typ) {
					found = append(found, readMessage(fset, lit))
					return false
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// isMessage reports whether typ names Message, from web, or in it when inWeb: another
// package's Message, such as the agent's, is not one.
func isMessage(typ ast.Expr, inWeb bool) bool {
	switch typ := typ.(type) {
	case *ast.Ident:
		return inWeb && typ.Name == "Message"
	case *ast.SelectorExpr:
		pkg, ok := typ.X.(*ast.Ident)
		return ok && pkg.Name == "web" && typ.Sel.Name == "Message"
	}
	return false
}

// readMessage reads a Message literal's En and It, which must be string literals so this
// test can read them.
func readMessage(fset *token.FileSet, e ast.Expr) foundMessage {
	m := foundMessage{pos: fset.Position(e.Pos()).String()}
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		m.err = "a Message is not a literal"
		return m
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			m.err = "a Message's fields are not named"
			return m
		}
		key, _ := kv.Key.(*ast.Ident)
		val, ok := kv.Value.(*ast.BasicLit)
		if key == nil || !ok || val.Kind != token.STRING {
			m.err = "a Message's texts are not string literals"
			return m
		}
		text, err := strconv.Unquote(val.Value)
		if err != nil {
			m.err = err.Error()
			return m
		}
		switch key.Name {
		case "En":
			m.en = text
		case "It":
			m.it = text
		}
	}
	return m
}
