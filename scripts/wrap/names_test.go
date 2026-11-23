package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// A Go comment writes its function's parameters, named results and receiver as `name`
// (docs/agents/coding-standards.md). Only a name that isn't a word can be told from the
// prose around it, so only those are checked; templ files aren't.
func TestCommentNames(t *testing.T) {
	err := walk("../..", func(path, src string) error {
		if filepath.Ext(path) != ".go" {
			return nil
		}
		found, err := bareNames(src)
		for _, f := range found {
			t.Errorf("%s:%s: write it as `name`", path, f)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// words are the short names in use that are also words, which a comment may use as the
// word: a name of five letters or fewer is checked unless it is one of them. id is one,
// as prose names a Run's or a Cycle's id beside an id of something else.
var words = strings.Fields(`a added again all asked at back beat block body brief call
	calls cause class code codes cycle day days eaten eats edit email ended ends fibre field
	file find first floor focus form forms found from going got group held hint holds hour
	id ids index input item items kept key kind kinds known label last like line lines link list
	logs lot lots meta month more moved name named names need next note notes now on only
	open own owned page past path per plan query reply rest rice root row rows run send set
	sheet show shown side size slots spans step steps stock stop store sugar tag terms text
	times title to today tone tool typed under unit until value view want wants what when
	where why words write zone`)

// checked reports whether `name` is unlike a word: with a capital or a digit, as runID,
// or short and not one of words, as ctx.
func checked(name string) bool {
	if strings.ContainsFunc(name, func(r rune) bool { return unicode.IsUpper(r) || unicode.IsDigit(r) }) {
		return true
	}
	return len(name) <= 5 && !slices.Contains(words, name)
}

// bareNames are the checked names `src`'s comments write bare, as "line: name", each in
// the doc comment of the function it names, or in a comment inside it.
func bareNames(src string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		var lists []*ast.FieldList
		var doc *ast.CommentGroup
		var body *ast.BlockStmt
		switch fn := n.(type) {
		case *ast.FuncDecl:
			lists = []*ast.FieldList{fn.Recv, fn.Type.TypeParams, fn.Type.Params, fn.Type.Results}
			doc, body = fn.Doc, fn.Body
		case *ast.FuncLit:
			lists = []*ast.FieldList{fn.Type.Params, fn.Type.Results}
			body = fn.Body
		default:
			return true
		}
		var groups []*ast.CommentGroup
		if doc != nil {
			groups = append(groups, doc)
		}
		for _, g := range f.Comments {
			if body != nil && g.Pos() > body.Lbrace && g.End() < body.Rbrace {
				groups = append(groups, g)
			}
		}
		for _, l := range lists {
			for _, field := range fieldsOf(l) {
				for _, name := range field.Names {
					if !checked(name.Name) {
						continue
					}
					for _, g := range groups {
						for _, c := range g.List {
							if isBare(c.Text, name.Name) {
								found = append(found, fmt.Sprintf("%d: %s", fset.Position(c.Pos()).Line, name.Name))
							}
						}
					}
				}
			}
		}
		return true
	})
	slices.Sort(found)
	return slices.Compact(found), nil
}

// fieldsOf is `l`'s fields, none when `l` is nil.
func fieldsOf(l *ast.FieldList) []*ast.Field {
	if l == nil {
		return nil
	}
	return l.List
}

var inBackticks = regexp.MustCompile("`[^`]*`")

// isBare reports whether `comment`, unless a directive, writes `name` as a word outside
// backticks: not part of a selector, a path or a string, nor the end of "don't".
func isBare(comment, name string) bool {
	if !isComment(comment) {
		return false
	}
	text := inBackticks.ReplaceAllString(comment, "``")
	re := regexp.MustCompile(`(^|[^\w.'’"{/=$#*@-])` + regexp.QuoteMeta(name) + `($|[^\w"}/=(-])`)
	return re.MatchString(text)
}

// The check itself: what it finds, and what it leaves alone.
func TestBareNames(t *testing.T) {
	// Each source follows "package p", so its first line is 2.
	cases := []struct {
		name, src string
		want      []string
	}{
		{"marked", "// f is `p` as is.\nfunc f(p int) {}", nil},
		{"bare", "// f is p as is.\nfunc f(p int) {}", []string{"2: p"}},
		{"a selector, marked", "// f is `p.X`.\nfunc f(p T) {}", nil},
		{"a word", "// f starts from now.\nfunc f(from, now int) {}", nil},
		{"a long name is a word", "// f is the normalised one.\nfunc f(normalised string) {}", nil},
		{"camel case", "// f runs runID.\nfunc f(runID int) {}", []string{"2: runID"}},
		{"a fragment", "// f links to #p.\nfunc f(p int) {}", nil},
		{"don't", "// f doesn't fail.\nfunc f(t T) {}", nil},
		{"a receiver", "// f is s.\nfunc (s S) f() {}", []string{"2: s"}},
		{"a result", "// f is n.\nfunc f() (n int) { return }", []string{"2: n"}},
		{"in the body", "func f(p int) {\n\t// p again\n\t_ = p\n}", []string{"3: p"}},
		{"a closure's", "var f = func(q int) {\n\t// with q\n\t_ = q\n}", []string{"3: q"}},
		{"another function's", "// g has p.\nfunc g() {}\nfunc f(p int) {}", nil},
		{"a directive", "func f(p int) {\n\t//nolint:p\n\t_ = p\n}", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := bareNames("package p\n" + c.src)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("bareNames = %q, want %q", got, c.want)
			}
		})
	}
}
