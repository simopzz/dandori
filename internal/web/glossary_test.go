package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ordinarySense are avoided words a page uses in their ordinary sense,
// not as the term the glossary avoids them for.
var ordinarySense = map[string]string{
	"log":  `"the server log", not a Trace`,
	"step": `a Recipe's steps, which the glossary names so, not a Trace Event`,
}

// Pages speak the glossary: what a template shows never uses a word
// GLOSSARY.md lists under _Avoid_. A glossary term containing such a word
// (Meal Out holds "meal") is the term, and fine; so is a word in
// ordinarySense.
func TestTemplatesAvoidWhatTheGlossaryAvoids(t *testing.T) {
	glossary, err := os.ReadFile("../../GLOSSARY.md")
	if err != nil {
		t.Fatal(err)
	}
	terms, avoided := glossaryWords(string(glossary))

	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".templ" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range visibleText(string(src)) {
			shown := line.text
			for _, term := range terms {
				shown = term.ReplaceAllString(shown, " ")
			}
			for word, re := range avoided {
				if re.MatchString(shown) {
					t.Errorf("%s line %d shows %q, which GLOSSARY.md avoids: %s", path, line.line, word, strings.TrimSpace(line.text))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

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
	for _, m := range glossaryTerm.FindAllStringSubmatch(glossary, -1) {
		terms = append(terms, word(m[1]))
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

// visibleText is the text a .templ file's templates show, line by line:
// what is left of their bodies once templ statements, tags and Go
// expressions are removed. Go code outside templates shows nothing.
func visibleText(src string) []textLine {
	var lines []textLine
	in, tag := false, false
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
