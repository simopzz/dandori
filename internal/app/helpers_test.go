package app_test

import (
	"html"
	"strings"
)

// text is the page's main content without markup, its whitespace collapsed.
func text(s string) string {
	if _, main, ok := strings.Cut(s, "<main"); ok {
		s, _, _ = strings.Cut(main, "</main>")
		s = "<main" + s
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(b.String())), " ")
}
