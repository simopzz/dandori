package apptest

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// FormData is what a form on a page submits, as a browser would.
type FormData struct {
	Action string
	Values url.Values
}

// Get returns the first value of field.
func (f FormData) Get(field string) string { return f.Values.Get(field) }

// Form reads the form with the given id from page: its action and the
// values of its inputs, textareas and selects as they are shown.
func Form(t testing.TB, page, id string) FormData {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	var form *html.Node
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "form" && attr(n, "id") == id {
			form = n
			break
		}
	}
	if form == nil {
		t.Fatalf("page has no form with id %q:\n%s", id, page)
	}

	f := FormData{Action: attr(form, "action"), Values: url.Values{}}
	for n := range form.Descendants() {
		if n.Type != html.ElementNode || attr(n, "name") == "" || inTemplate(n, form) {
			continue
		}
		name := attr(n, "name")
		switch n.Data {
		case "input":
			switch attr(n, "type") {
			case "submit":
			case "radio", "checkbox":
				if hasAttr(n, "checked") {
					f.Values.Add(name, attr(n, "value"))
				}
			default:
				f.Values.Add(name, attr(n, "value"))
			}
		case "textarea":
			var b strings.Builder
			for c := range n.ChildNodes() {
				b.WriteString(c.Data)
			}
			// Browsers drop one newline right after <textarea>.
			f.Values.Add(name, strings.TrimPrefix(b.String(), "\n"))
		case "select":
			var first, selected string
			found, firstSet := false, false
			for o := range n.Descendants() {
				if o.Type != html.ElementNode || o.Data != "option" {
					continue
				}
				v := optionValue(o)
				if !firstSet {
					first, firstSet = v, true
				}
				if hasAttr(o, "selected") {
					selected, found = v, true
				}
			}
			if !found {
				selected = first
			}
			f.Values.Add(name, selected)
		}
	}
	return f
}

// inTemplate reports whether n is template content, which isn't submitted.
func inTemplate(n, form *html.Node) bool {
	for p := n.Parent; p != form; p = p.Parent {
		if p.Data == "template" {
			return true
		}
	}
	return false
}

func optionValue(o *html.Node) string {
	if hasAttr(o, "value") {
		return attr(o, "value")
	}
	var b strings.Builder
	for c := range o.ChildNodes() {
		b.WriteString(c.Data)
	}
	return strings.TrimSpace(b.String())
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
