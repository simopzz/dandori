package web

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The type floor: nothing a User reads to act is under 0.875rem, and letter-spacing
// is for the eyebrow, the wordmark and the titles in the pixel face. See docs/design.md.
func TestTypeHasAFloorAndNoTrackingOnWords(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	small := regexp.MustCompile(`\.eyebrow|\.wordmark`)
	tracked := regexp.MustCompile(`^(body|h1|h2|(legend)?\.section-heading|\.eyebrow|(.* )?\.wordmark[\w-]*)$`)
	size := regexp.MustCompile(`(\d*\.?\d+)(rem|em|px)\b`)
	for _, r := range cssRules(string(css)) {
		for _, d := range r.decls {
			switch d.prop {
			case "font-size", "font":
				if small.MatchString(r.selector) {
					continue
				}
				for _, m := range size.FindAllStringSubmatch(d.value, -1) {
					n, _ := strconv.ParseFloat(m[1], 64)
					if m[2] == "px" {
						n /= 16
					}
					if n < 0.875 {
						t.Errorf("app.css line %d: %s has %s: %s, under the 0.875rem floor", d.line, r.selector, d.prop, d.value)
					}
				}
			case "letter-spacing":
				if d.value == "normal" {
					continue
				}
				for _, sel := range strings.Split(r.selector, ",") {
					if !tracked.MatchString(strings.TrimSpace(sel)) {
						t.Errorf("app.css line %d: %s has letter-spacing: %s; only the eyebrow, the wordmark and pixel titles are tracked", d.line, sel, d.value)
					}
				}
			}
		}
	}
}

type cssRule struct {
	selector string
	decls    []cssDecl
}

type cssDecl struct {
	line        int
	prop, value string
}

// cssRules are app.css's innermost rules, a media query's included, comments removed.
func cssRules(css string) []cssRule {
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllStringFunc(css, func(c string) string {
		return regexp.MustCompile(`[^\n]`).ReplaceAllString(c, " ")
	})
	var rules []cssRule
	for _, at := range regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`).FindAllStringSubmatchIndex(css, -1) {
		r := cssRule{selector: strings.Join(strings.Fields(css[at[2]:at[3]]), " ")}
		line := strings.Count(css[:at[4]], "\n") + 1
		for _, d := range strings.Split(css[at[4]:at[5]], ";") {
			prop, value, ok := strings.Cut(d, ":")
			if ok {
				r.decls = append(r.decls, cssDecl{line + strings.Count(prop, "\n"), strings.TrimSpace(prop), strings.TrimSpace(value)})
			}
			line += strings.Count(d, "\n")
		}
		rules = append(rules, r)
	}
	return rules
}
