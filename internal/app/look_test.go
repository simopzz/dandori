package app_test

import (
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// A page numbers its own groups: the Recipe Library, a Recipe, a Run and the Ingredients
// page. The square is for a side sheet's, such as the Ingredients sheet's.
func TestAPageNumbersItsOwnGroups(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	recipe := s.Page().Path
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)
	run := s.Page().Path

	for path, want := range map[string]string{
		"/recipes":     "01 Recipe Drafts to check Lasagne",
		recipe:         "01 Per Portion",
		run:            "01 Trace",
		"/ingredients": "01 Your Ingredients",
	} {
		s.Open(path)
		s.Sees(want)
		if strings.Contains(s.Page().Body, `class="mark"`) {
			t.Errorf("%s marks one of its own groups with the square", path)
		}
	}
	s.Open("/recipes")
	s.Sees("02 Recipes")
	s.Open(recipe)
	s.Sees("02 Ingredients rice 200 g")
	s.Sees("03 Steps")

	if sheet := s.HTMXGet(t, "/ingredients/sheet"); !strings.Contains(sheet.Body, `class="mark"`) || strings.Contains(sheet.Body, `class="num"`) {
		t.Errorf("the Ingredients side sheet numbers its groups, want the square:\n%s", sheet.Body)
	}
}

// A title sets its times, quantities and User text in the body face, where l, I and 1
// are told apart: Today's time of day, a Recipe's name.
func TestATitleSetsItsDataInTheBodyFace(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 12:00")
	s.Open("/")
	if !strings.Contains(s.Page().Body, `<span class="data">1:00 pm</span>`) {
		t.Errorf("Today's title sets its time in the pixel face:\n%s", s.Page().Body)
	}
	s.WritesRecipe(riceBowl())
	if !strings.Contains(s.Page().Body, `<h1><span class="data">Rice bowl</span></h1>`) {
		t.Errorf("a Recipe's title sets its name in the pixel face:\n%s", s.Page().Body)
	}
}

// On a phone the full sky is Today's: every other page has a slim strip of it, so its
// first content, after the page's header, sits within 300px of the top.
func TestBrowserTheSkyIsSlimOnEveryPageButToday(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	if err := b.SetViewportSize(390, 844); err != nil {
		t.Fatalf("size the page as a phone: %v", err)
	}
	b.SignUp(t, "ada@example.com")

	b.Open(t, "/")
	if got := measure(t, b, `document.querySelector(".masthead").getBoundingClientRect().height`); got < 150 {
		t.Errorf("Today's sky is %vpx tall, want the full sky", got)
	}
	for _, path := range []string{"/recipes", "/kitchen", "/ingredients", "/profile", "/runs"} {
		b.Open(t, path)
		if got := measure(t, b, `document.querySelector("main .page-header").nextElementSibling.getBoundingClientRect().top`); got > 300 {
			t.Errorf("%s's first content starts %vpx down, want at most 300", path, got)
		}
	}
}

// measure is the number script works out on b's page.
func measure(t *testing.T, b *apptest.Browser, script string) float64 {
	t.Helper()
	got, err := b.Evaluate("() => " + script)
	if err != nil {
		t.Fatalf("measure %s: %v", script, err)
	}
	switch v := got.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	t.Fatalf("measure %s: %v is not a number", script, got)
	return 0
}
