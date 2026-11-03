package apptest

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Link returns where the first link on page whose text contains text
// leads, as a user would follow it.
func Link(t testing.TB, page, text string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "a" {
			continue
		}
		var shown strings.Builder
		for c := range n.Descendants() {
			if c.Type == html.TextNode {
				shown.WriteString(c.Data)
			}
		}
		if strings.Contains(strings.Join(strings.Fields(shown.String()), " "), text) {
			return attr(n, "href")
		}
	}
	t.Fatalf("page has no link showing %q", text)
	return ""
}
