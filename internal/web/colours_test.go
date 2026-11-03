package web

import (
	"encoding/json"
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

// The browser chrome's colour can't read a token, so themeColour and the
// manifest repeat the paper colour; they must match it.
func TestThemeColourIsThePaper(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`--color-bg: (#[0-9a-fA-F]+);`).FindSubmatch(css)
	if m == nil {
		t.Fatal("app.css has no --color-bg token")
	}
	paper := string(m[1])

	var manifestColours struct {
		Theme      string `json:"theme_color"`
		Background string `json:"background_color"`
	}
	if err := json.Unmarshal(manifest, &manifestColours); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]string{
		"themeColour":                 themeColour,
		"manifest's theme_color":      manifestColours.Theme,
		"manifest's background_color": manifestColours.Background,
	} {
		if got != paper {
			t.Errorf("%s = %s, want --color-bg %s", name, got, paper)
		}
	}
}

// tokensRemoved blanks the contents of every :root block, including one
// inside a media query (dark mode), keeping their lines so line numbers
// still match.
func tokensRemoved(t *testing.T, css string) string {
	blocks := regexp.MustCompile(`:root \{[^}]*\}`)
	if !blocks.MatchString(css) {
		t.Fatal("app.css has no :root token block")
	}
	return blocks.ReplaceAllStringFunc(css, func(block string) string {
		return regexp.MustCompile(`[^\n]`).ReplaceAllString(block, "")
	})
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
