package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// A User's first sign-in asks for their Language before anything else. Picking Italian
// makes the pages Italian, and the next sign-in doesn't ask again.
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

// The Language can be changed on the Profile, while the User has written no Food Term,
// confirmed in the Language chosen.
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
	s.IsConfirmed("Profilo salvato.")
	s.Open("/")
	s.Sees("lunedì 2 novembre")

	s.Open("/profile")
	s.IsNotConfirmed()
	s.ChangesLanguage("English")
	s.IsConfirmed("Profile saved.")
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

// Pages before signing in are in English, with a switch to Italian that the following
// pages remember, up to the Language choice.
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

// A Language Dandori doesn't speak is refused, with the choice asked again.
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

// A change the Profile refuses comes back with the Language as it was.
func TestTheProfileRefusesAnotherLanguageKeepingTheUsers(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")

	form := apptest.Form(t, h.Get(t, "/profile").Body, "profile-language")
	form.Values.Set("language", "fr")
	page := h.PostForm(t, form.Action, form.Values)
	if page.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("changing to fr answered %d, want 422:\n%s", page.StatusCode, page.Body)
	}
	if got := apptest.Form(t, page.Body, "profile-language").Get("language"); got != "en" {
		t.Errorf("the refused Profile has %q chosen, want en, the User's", got)
	}
}

// The Language is chosen once at the first sign-in: the choice, posted again, goes to
// Today and changes nothing; changing it is the Profile's.
func TestTheLanguageIsChosenOnce(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	choice := s.Page()
	s.ChoosesLanguage("English")

	page := s.ChooseLanguage(t, choice, "Italiano")
	if page.Path != "/" {
		t.Errorf("choosing again landed on %s, want /", page.Path)
	}
	s.Open("/")
	s.Sees("Monday 2 November")
}

// Once the User has added an Ingredient, their Language stays: the Profile refuses a
// change, keeping the one they have.
func TestTheLanguageStaysOnceTheUserHasAnIngredient(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{
		Name: "egg", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6.3", Carbs: "0.4", Fat: "4.8",
	})

	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.IsRefused()
	s.Sees("Your kitchen already holds Ingredients or Recipes in this Language, so it stays.")
	s.Open("/")
	s.Sees("Monday 2 November")
}

// The Language switch opens a page of this site again, whatever slashes its path begins
// with: a browser reads a location beginning with // as another site's.
func TestTheLanguageSwitchStaysOnThisSite(t *testing.T) {
	h := apptest.Start(t)
	unfollowed := *h.Client
	unfollowed.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := unfollowed.Get(h.URL + "//evil.example/sign-in?lang=it")
	if err != nil {
		t.Fatalf("open the Language switch: %v", err)
	}
	res.Body.Close()

	if got := res.Header.Get("Location"); !strings.HasPrefix(got, "/") || strings.HasPrefix(got, "//") {
		t.Errorf("the Language switch redirects to %q, want a path on this site", got)
	}
	if !slices.ContainsFunc(res.Cookies(), func(c *http.Cookie) bool { return c.Value == "it" }) {
		t.Errorf("the Language switch set no Language cookie: %v", res.Cookies())
	}
}
