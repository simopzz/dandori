package apptest

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Anchored returns the text shown by the element of `page` whose `id` is `id`, as Text
// reads it: where a link to #id lands.
func Anchored(t testing.TB, page, id string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && attr(n, "id") == id {
			var b strings.Builder
			if err := html.Render(&b, n); err != nil {
				t.Fatalf("render #%s: %v", id, err)
			}
			return Text(b.String())
		}
	}
	t.Fatalf("page has no element whose id is %q", id)
	return ""
}

// Link returns where the first link on `page` whose text contains `text`
// leads, as a user would follow it: without JavaScript, so one in a noscript too.
func Link(t testing.TB, page, text string) string {
	t.Helper()
	doc, err := html.ParseWithOptions(strings.NewReader(page), html.ParseOptionEnableScripting(false))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "a" && strings.Contains(shownText(n), text) {
			return attr(n, "href")
		}
	}
	t.Fatalf("page has no link showing %q", text)
	return ""
}

// SheetSource returns the fragment the first link on `page` whose text contains `text`
// loads into its sheet, as htmx follows it: a SideSheetLink's src.
func SheetSource(t testing.TB, page, text string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "a" && hasAttr(n, "hx-get") && strings.Contains(shownText(n), text) {
			return attr(n, "hx-get")
		}
	}
	t.Fatalf("page has no link opening a sheet showing %q", text)
	return ""
}

// CurrentLink is the text of the link on `page` marked aria-current="true", the one shown
// of those a page switches between. A tab, marked "page", isn't one.
func CurrentLink(t testing.TB, page string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "a" && attr(n, "aria-current") == "true" {
			return shownText(n)
		}
	}
	t.Fatalf("page has no link marked current")
	return ""
}
