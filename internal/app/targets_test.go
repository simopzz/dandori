package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

var targets = apptest.Targets{Kcal: "2200", Protein: "130", Fibre: "30", Sugar: "50", Tolerance: "8"}

// profileOf signs up email, in English, on a Profile page.
func profileOf(t *testing.T, email string) *apptest.Scenario {
	s := apptest.NewScenario(t)
	s.Invite(email)
	s.SignsIn(email)
	s.ChoosesLanguage("English")
	s.Open("/profile")
	return s
}

// shownTargets are the Targets the Targets form on page holds.
func shownTargets(t *testing.T, page string) apptest.Targets {
	t.Helper()
	f := apptest.Form(t, page, "profile-targets")
	return apptest.Targets{Kcal: f.Get("kcal"), Protein: f.Get("protein"), Fibre: f.Get("fibre"), Sugar: f.Get("sugar"), Tolerance: f.Get("tolerance")}
}

// A new User has no Targets, and the Profile says planning needs them, with the calorie
// tolerance at 5% and carbs and fat named as never aimed at.
func TestANewUserHasNoTargets(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Sees("Targets")
	s.Sees("Planning needs your Targets.")
	s.Sees("Carbs and fat have no Target: they are shown, never aimed at.")
	if got, want := shownTargets(t, s.Page().Body), (apptest.Targets{Tolerance: "5"}); got != want {
		t.Errorf("a new User's Targets are %+v, want %+v", got, want)
	}
}

// Saved Targets are shown back, confirmed, and planning no longer asks for them.
func TestTargetsAreSavedAndShownBack(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.SavesTargets(targets)
	s.IsAt("/profile")
	s.IsConfirmed("Profile saved.")
	s.DoesNotSee("Planning needs your Targets.")
	if got := shownTargets(t, s.Page().Body); got != targets {
		t.Errorf("the saved Targets show %+v, want %+v", got, targets)
	}

	s.Open("/profile")
	if got := shownTargets(t, s.Page().Body); got != targets {
		t.Errorf("the Targets reopened show %+v, want %+v", got, targets)
	}
	changed := targets
	changed.Kcal = "1900"
	s.SavesTargets(changed)
	if got := shownTargets(t, s.Page().Body); got != changed {
		t.Errorf("the changed Targets show %+v, want %+v", got, changed)
	}
}

// Each refused value is refused on its own field, saving nothing, with the rest of what
// was typed kept.
func TestEachRefusedTargetIsRefusedOnItsField(t *testing.T) {
	cases := []struct {
		name   string
		change func(*apptest.Targets)
		shown  string
	}{
		{"missing", func(ts *apptest.Targets) { ts.Kcal = "" }, "Calories (kcal a day) Required."},
		{"zero", func(ts *apptest.Targets) { ts.Protein = "0" }, "Protein (g, at least) Must be above 0."},
		{"negative", func(ts *apptest.Targets) { ts.Fibre = "-4" }, "Fibre (g, at least) Must be above 0."},
		{"not whole", func(ts *apptest.Targets) { ts.Sugar = "12.5" }, "Sugar (g, at most) Type a whole number."},
		{"too large", func(ts *apptest.Targets) { ts.Kcal = "10000" }, "Calories (kcal a day) Must be below 10000."},
		{"tolerance missing", func(ts *apptest.Targets) { ts.Tolerance = "" }, "Tolerance (± %) Required."},
		{"tolerance below", func(ts *apptest.Targets) { ts.Tolerance = "0" }, "Tolerance (± %) From 1 to 20."},
		{"tolerance above", func(ts *apptest.Targets) { ts.Tolerance = "21" }, "Tolerance (± %) From 1 to 20."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := profileOf(t, "ada@example.com")
			typed := targets
			c.change(&typed)
			s.SavesTargets(typed)
			if s.Page().StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("the refused Targets answered %d, want 422", s.Page().StatusCode)
			}
			s.Sees(c.shown)
			s.IsNotConfirmed()
			if got := shownTargets(t, s.Page().Body); got != typed {
				t.Errorf("the refused Targets hold %+v, want what was typed, %+v", got, typed)
			}
			s.Open("/profile")
			s.Sees("Planning needs your Targets.")
		})
	}
}

// Posted by htmx, the Targets section answers with itself: saved with a Notice, or
// refused as typed; a plain post lands on the Profile page.
func TestTheTargetsSectionAnswersHTMXWithItself(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	form := apptest.Form(t, s.Page().Body, "profile-targets")
	refusedTargets := targets
	refusedTargets.Tolerance = "25"
	refusedTargets.Fill(form)

	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("the refused Targets answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if got := shownTargets(t, refused.Body); got != refusedTargets {
		t.Errorf("the refused Targets hold %+v, want %+v", got, refusedTargets)
	}
	if shown := apptest.Text(refused.Body); !strings.Contains(shown, "From 1 to 20.") {
		t.Errorf("the refused Targets show %q, want the tolerance refused", shown)
	}
	if notices := apptest.Notices(t, refused.Body); len(notices) > 0 {
		t.Errorf("the refused Targets confirm %q", notices)
	}

	targets.Fill(form)
	saved := s.HTMXPostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") {
		t.Fatalf("saving the Targets answered %d, want 200 and a fragment:\n%s", saved.StatusCode, saved.Body)
	}
	if got := shownTargets(t, saved.Body); got != targets {
		t.Errorf("the saved Targets hold %+v, want %+v", got, targets)
	}
	if notices := apptest.Notices(t, saved.Body); !slices.Equal(notices, []string{"Profile saved."}) {
		t.Errorf("saving the Targets confirms %q", notices)
	}
	if shown := apptest.Text(saved.Body); strings.Contains(shown, "Planning needs your Targets.") {
		t.Errorf("the saved Targets still say planning needs them: %s", shown)
	}
}

// Each User sees only their own Targets.
func TestEachUserHasTheirOwnTargets(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.SavesTargets(targets)
	s.SignsOut()

	s.Invite("bea@example.com")
	s.SignsIn("bea@example.com")
	s.ChoosesLanguage("English")
	s.Open("/profile")
	s.Sees("Planning needs your Targets.")
	if got, want := shownTargets(t, s.Page().Body), (apptest.Targets{Tolerance: "5"}); got != want {
		t.Errorf("another User's Targets are %+v, want %+v", got, want)
	}

	s.SwitchesTo("ada@example.com")
	s.Open("/profile")
	if got := shownTargets(t, s.Page().Body); got != targets {
		t.Errorf("the first User's Targets are %+v, want %+v", got, targets)
	}
}

// Saving the Targets through a real browser swaps the section in place: a refused field
// gets the focus, and saved ones are confirmed in a floating Notice.
func TestBrowserSavesTheTargetsInPlace(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/profile")
	b.Sees(t, "Planning needs your Targets.")
	if _, err := b.Evaluate("window.stayed = true"); err != nil {
		t.Fatalf("mark the page: %v", err)
	}

	section := b.Locator("#profile-targets")
	field := func(name string) playwright.Locator {
		return section.GetByRole("spinbutton", playwright.LocatorGetByRoleOptions{Name: name})
	}
	for name, v := range map[string]string{"Calories": "2200", "Protein": "130", "Fibre": "30", "Sugar": "50", "Tolerance": "25"} {
		if err := field(name).Fill(v); err != nil {
			t.Fatalf("fill %s: %v", name, err)
		}
	}
	save := section.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save"})
	if err := save.Click(); err != nil {
		t.Fatalf("save the Targets: %v", err)
	}
	expect := playwright.NewPlaywrightAssertions()
	tolerance := field("Tolerance")
	if err := expect.Locator(tolerance).ToHaveAttribute("aria-invalid", "true"); err != nil {
		t.Fatalf("the tolerance isn't refused: %v", err)
	}
	if err := expect.Locator(tolerance).ToBeFocused(); err != nil {
		t.Errorf("the refused tolerance isn't focused: %v", err)
	}
	b.Sees(t, "From 1 to 20.")

	if err := tolerance.Fill("8"); err != nil {
		t.Fatalf("fill the tolerance: %v", err)
	}
	if err := save.Click(); err != nil {
		t.Fatalf("save the Targets: %v", err)
	}
	b.IsConfirmed(t, "Profile saved.")
	if err := expect.Locator(section).Not().ToContainText("Planning needs your Targets."); err != nil {
		t.Errorf("the saved Targets still say planning needs them: %v", err)
	}
	if err := expect.Locator(field("Calories")).ToHaveValue("2200"); err != nil {
		t.Errorf("the saved Targets don't show the calories: %v", err)
	}
	if stayed, err := b.Evaluate("window.stayed === true"); err != nil || stayed != true {
		t.Errorf("saving the Targets left the page: %v, %v", stayed, err)
	}
	b.Shot(t, "targets")
}
