package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// colourLiteral is a hex or rgb()/rgba() colour value.
var colourLiteral = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\brgba?\(`)

// Colours are tokens: a literal colour value belongs only in the :root
// block that opens app.css, never in a template. See docs/design.md.
func TestColourValuesAreOnlyTokens(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range colourLiterals(tokensRemoved(t, string(css))) {
		t.Errorf("app.css line %d has a colour literal outside the token block: %s", at.line, at.text)
	}

	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".templ" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, at := range colourLiterals(string(src)) {
			t.Errorf("%s line %d has a colour literal; use a token from app.css: %s", path, at.line, at.text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// tokensRemoved blanks the first :root block's contents, keeping its lines
// so line numbers still match.
func tokensRemoved(t *testing.T, css string) string {
	start := strings.Index(css, ":root {")
	if start < 0 {
		t.Fatal("app.css has no :root token block")
	}
	end := start + strings.Index(css[start:], "\n}")
	if end < start {
		t.Fatal("app.css's :root token block is not closed")
	}
	blank := regexp.MustCompile(`[^\n]`).ReplaceAllString(css[start:end], "")
	return css[:start] + blank + css[end:]
}

type literal struct {
	line int
	text string
}

func colourLiterals(src string) []literal {
	var found []literal
	for i, line := range strings.Split(src, "\n") {
		if m := colourLiteral.FindString(line); m != "" {
			found = append(found, literal{i + 1, strings.TrimSpace(line)})
		}
	}
	return found
}
