package web

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// ordinarySense are avoided words a page uses in their ordinary sense,
// not as the term the glossary avoids them for.
var ordinarySense = map[string]string{
	"log":  `"the server log", not a Trace`,
	"step": `a Recipe's steps, which the glossary names so, not a Trace Event`,
}

// nameSense are names that use an avoided word in its ordinary sense, not as the term
// the glossary avoids it for.
var nameSense = map[string]string{
	"Schedule":            "starting the app's scheduler, not a Plan",
	"goneToTodayIfChosen": "the verb, not a Stock Item left in the kitchen",
	"settings":            "the test server's own settings, not the Profile",
	"TestStartupMigratesTheDatabaseBeforeServing": "serving HTTP, not a Portion",
}

// Pages speak the glossary: what a page shows, a Message's English or a template's
// own text, never uses a word GLOSSARY.md lists under _Avoid_. A glossary term
// containing such a word (Meal Out holds "meal") is the term, and fine; so is a word
// in ordinarySense.
func TestPagesAvoidWhatTheGlossaryAvoids(t *testing.T) {
	glossary, err := os.ReadFile("../../GLOSSARY.md")
	if err != nil {
		t.Fatal(err)
	}
	terms, avoided := glossaryWords(string(glossary))
	check := func(where, text string) {
		shown := text
		for _, term := range terms {
			shown = term.ReplaceAllString(shown, " ")
		}
		for word, re := range avoided {
			if re.MatchString(shown) {
				t.Errorf("%s shows %q, which GLOSSARY.md avoids: %s", where, word, strings.TrimSpace(text))
			}
		}
	}

	for _, m := range messages(t) {
		check(m.pos, m.en)
	}
	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".templ" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range visibleText(string(src)) {
			check(fmt.Sprintf("%s line %d", path, line.line), line.text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Code speaks the glossary too (docs/agents/domain.md): the names the code declares at
// the top level, and its test names, read word by word, never use a word GLOSSARY.md
// lists under _Avoid_, unless nameSense allows it.
func TestNamesAvoidWhatTheGlossaryAvoids(t *testing.T) {
	glossary, err := os.ReadFile("../../GLOSSARY.md")
	if err != nil {
		t.Fatal(err)
	}
	terms, avoided := glossaryWords(string(glossary))
	fset := token.NewFileSet()
	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_templ.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, name := range declaredNames(file) {
			if _, ok := nameSense[name.Name]; ok {
				continue
			}
			shown := splitName(name.Name)
			for _, term := range terms {
				shown = term.ReplaceAllString(shown, " ")
			}
			for word, re := range avoided {
				if re.MatchString(shown) {
					t.Errorf("%s: %s uses %q, which GLOSSARY.md avoids", fset.Position(name.Pos()), name.Name, word)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// declaredNames are the functions, methods, types, variables and constants file declares
// at its top level.
func declaredNames(file *ast.File) []*ast.Ident {
	var names []*ast.Ident
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			names = append(names, d.Name)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, s.Name)
				case *ast.ValueSpec:
					names = append(names, s.Names...)
				}
			}
		}
	}
	return names
}

// splitName is a Go name as words: TestAStockItem reads "Test A Stock Item".
func splitName(name string) string {
	return wordBreak.ReplaceAllString(acronymBreak.ReplaceAllString(name, "$1 $2"), "$1 $2")
}

var (
	wordBreak    = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	acronymBreak = regexp.MustCompile(`([A-Z])([A-Z][a-z])`)
)

var (
	glossaryTerm  = regexp.MustCompile(`(?m)^\*\*(.+?)\*\*:`)
	glossaryAvoid = regexp.MustCompile(`(?m)^_Avoid_: (.+)$`)
)

// glossaryWords are the glossary's terms and the words it avoids, each as
// a whole-word, case-insensitive pattern; a term matches its plural too.
func glossaryWords(glossary string) (terms []*regexp.Regexp, avoided map[string]*regexp.Regexp) {
	word := func(w string) *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `s?\b`)
	}
	names := []string{}
	for _, m := range glossaryTerm.FindAllStringSubmatch(glossary, -1) {
		names = append(names, m[1])
	}
	// Longest first, so a term holding another (Trace Event holds Trace) is taken whole.
	slices.SortStableFunc(names, func(a, b string) int { return len(b) - len(a) })
	for _, name := range names {
		terms = append(terms, word(name))
	}
	avoided = map[string]*regexp.Regexp{}
	for _, m := range glossaryAvoid.FindAllStringSubmatch(glossary, -1) {
		for w := range strings.SplitSeq(m[1], ",") {
			w = strings.ToLower(strings.TrimSpace(w))
			if _, ok := ordinarySense[w]; !ok {
				avoided[w] = word(w)
			}
		}
	}
	return terms, avoided
}

type textLine struct {
	line int
	text string
}

var (
	templOpen   = regexp.MustCompile(`^templ \w+\(.*\) \{$`)
	templCode   = regexp.MustCompile(`^(@|if |} else|else |for |switch |case |default:|}|//)`)
	markup      = regexp.MustCompile(`<[^>]*>`)
	expressions = regexp.MustCompile(`\{[^}]*\}`)
)

// visibleText is the text a .templ file's templates show, line by line: what is left of
// their bodies once templ statements, tags, scripts and Go expressions are removed. Go
// code outside templates shows nothing.
func visibleText(src string) []textLine {
	var lines []textLine
	in, tag, script := false, false, false
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case templOpen.MatchString(line):
			in = true
			continue
		case !in:
			continue
		case raw == "}":
			in = false
			continue
		case script:
			script = !strings.Contains(line, "</script>")
			continue
		case strings.HasPrefix(line, "<script") && !strings.Contains(line, "</script>"):
			script = true
			continue
		}
		// A tag spread over several lines, its attributes one per line.
		if tag {
			if j := strings.Index(line, ">"); j >= 0 {
				line, tag = line[j+1:], false
			} else {
				continue
			}
		}
		if templCode.MatchString(line) {
			continue
		}
		line = markup.ReplaceAllString(line, " ")
		if j := strings.LastIndex(line, "<"); j >= 0 {
			line, tag = line[:j], true
		}
		if line = strings.TrimSpace(expressions.ReplaceAllString(line, " ")); line != "" {
			lines = append(lines, textLine{line: i + 1, text: line})
		}
	}
	return lines
}
