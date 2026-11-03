package apptest

import (
	"html"
	"strings"
)

// Text is the page's main content without markup, its whitespace
// collapsed: what a test checks a page shows.
func Text(page string) string {
	if _, main, ok := strings.Cut(page, "<main"); ok {
		page, _, _ = strings.Cut(main, "</main>")
		page = "<main" + page
	}
	var b strings.Builder
	inTag := false
	for _, r := range page {
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
