package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// The type floor on a phone, in English and Italian: the bottom tabs, a Profile
// FieldRow and the Recipe form's Ingredient grid still fit the screen.
func TestBrowserTypeFloorFitsAPhone(t *testing.T) {
	for _, lang := range []string{"English", "Italiano"} {
		t.Run(lang, func(t *testing.T) {
			h := apptest.Start(t)
			h.Invite("ada@example.com")
			b := h.Browser(t)
			if err := b.SetViewportSize(390, 844); err != nil {
				t.Fatalf("size the page as a phone: %v", err)
			}
			b.SignUp(t, "ada@example.com")
			b.Open(t, "/profile")
			b.ChooseLanguage(t, lang, "Save Language")
			b.IsAt(t, "/profile")
			fits(t, b, ".tab")
			fits(t, b, ".field-row")
			b.Open(t, "/recipes/new")
			fits(t, b, ".ingredient-grid")
		})
	}
}

// fits fails for each element matching selector, or inside one, whose content is
// wider than it or that runs past the screen's edge.
func fits(t *testing.T, b *apptest.Browser, selector string) {
	t.Helper()
	got, err := b.Evaluate(`sel => {
		const all = [...document.querySelectorAll(sel)];
		if (all.length === 0) return ["no " + sel];
		return all.flatMap(el => [el, ...el.querySelectorAll("*")])
			.filter(el => el.scrollWidth > el.clientWidth + 1 || el.getBoundingClientRect().right > innerWidth + 1)
			.map(el => el.outerHTML.slice(0, 120));
	}`, selector)
	if err != nil {
		t.Fatalf("measure %s: %v", selector, err)
	}
	for _, el := range got.([]any) {
		t.Errorf("%s overflows a phone: %v", selector, el)
	}
}
