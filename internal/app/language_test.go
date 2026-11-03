package app_test

import (
	"net/http"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// A User's first sign-in asks for their Language before anything else.
// Picking Italian makes the pages Italian, and the next sign-in doesn't ask
// again.
func TestTheFirstSignInAsksForTheLanguage(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")

	s.SignsIn("ada@example.com")
	s.IsAt("/language")
	s.Open("/")
	s.IsAt("/language")

	s.ChoosesLanguage("Italiano")
	s.IsAt("/")
	s.Sees("lunedì 2 novembre")

	s.Open("/profile")
	s.SignsOut()
	s.SignsIn("ada@example.com")
	s.IsAt("/")
	s.Sees("lunedì 2 novembre")
}

// The Language can be changed on the Profile, while the kitchen holds no
// food term.
func TestTheLanguageCanBeChangedOnTheProfile(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")

	s.Open("/profile")
	s.Sees("Profile")
	s.ChangesLanguage("Italiano")
	s.IsAt("/profile")
	s.Sees("Profilo")
	s.Open("/")
	s.Sees("lunedì 2 novembre")
}

// Each User has their own Language.
func TestEachUserHasTheirOwnLanguage(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.Invite("bea@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/profile")
	s.SignsOut()

	s.SignsIn("bea@example.com")
	s.IsAt("/language")
	s.ChoosesLanguage("English")
	s.Sees("Monday 2 November")
}

// Pages before signing in are in English, with a switch to Italian that
// the following pages remember, up to the Language choice.
func TestPagesBeforeSignInSwitchToItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")

	s.Open("/sign-in")
	s.Sees("Sign in")
	s.Follows("Italiano")
	s.IsAt("/sign-in")
	s.Sees("Accedi")

	s.AsksForALink("ada@example.com")
	s.IsAt("/sign-in/sent")
	s.Sees("Controlla la tua email")

	s.SignsIn("ada@example.com")
	s.IsAt("/language")
	s.Sees("Scegli la tua lingua")
	if got := apptest.Form(t, s.Page().Body, "language").Get("language"); got != "it" {
		t.Errorf("the Language choice has %q chosen, want it, as the pages before it", got)
	}
}

// A Language Dandori doesn't speak is refused, with the choice asked
// again.
func TestTheLanguageChoiceRefusesAnotherLanguage(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	page := h.SignIn(t, "ada@example.com")

	form := apptest.Form(t, page.Body, "language")
	form.Values.Set("language", "fr")
	page = h.PostForm(t, form.Action, form.Values)
	if page.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("choosing fr answered %d, want 422:\n%s", page.StatusCode, page.Body)
	}
	if page = h.Get(t, "/"); page.Path != "/language" {
		t.Errorf("after refusing fr, / opens %s, want the Language choice", page.Path)
	}
}
