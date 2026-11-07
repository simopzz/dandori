package apptest

import (
	"html"
	"strings"
	"testing"
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

// Notices are the texts of the floating Notices a page or a fragment shows, newest
// first, without markup: what a test checks a write confirmed.
func Notices(t testing.TB, page string) []string {
	t.Helper()
	var notices []string
	for {
		_, rest, ok := strings.Cut(page, `class="notice-text"`)
		if !ok {
			return notices
		}
		_, rest, ok = strings.Cut(rest, ">")
		if !ok {
			t.Fatalf("a Notice's text has no end to its tag:\n%s", page)
		}
		text, after, ok := strings.Cut(rest, "</p>")
		if !ok {
			t.Fatalf("a Notice's text has no </p>:\n%s", page)
		}
		notices = append(notices, Text(text))
		page = after
	}
}
