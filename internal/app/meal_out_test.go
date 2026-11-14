package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

var defaultMealOut = apptest.MealOut{Kcal: "1000", Protein: "35", Carbs: "110", Fat: "45", Fibre: "6", Sugar: "12"}

// mealOutOnForm is the Meal Out estimate the Meal Out form on page holds.
func mealOutOnForm(t *testing.T, page string) apptest.MealOut {
	t.Helper()
	f := apptest.Form(t, page, "profile-meal-out")
	return apptest.MealOut{
		Kcal: f.Get("kcal"), Protein: f.Get("protein"), Carbs: f.Get("carbs"),
		Fat: f.Get("fat"), Fibre: f.Get("fibre"), Sugar: f.Get("sugar"),
	}
}

// A new User's Profile shows the default Meal Out estimate.
func TestANewUserHasTheDefaultMealOutEstimate(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Sees("Meal Out estimate")
	if got := mealOutOnForm(t, s.Page().Body); got != defaultMealOut {
		t.Errorf("a new User's Meal Out estimate is %+v, want %+v", got, defaultMealOut)
	}
}

// A saved Meal Out estimate is shown back, and fibre and sugar left blank stay unknown,
// never 0.
func TestTheMealOutEstimateIsSavedWithFibreAndSugarGivenOrUnknown(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	given := apptest.MealOut{Kcal: "850", Protein: "40", Carbs: "0", Fat: "30", Fibre: "0", Sugar: "9"}
	s.SavesMealOut(given)
	s.IsAt("/profile")
	s.IsConfirmed("Profile saved.")
	if got := mealOutOnForm(t, s.Page().Body); got != given {
		t.Errorf("the saved Meal Out estimate shows %+v, want %+v", got, given)
	}

	unknown := apptest.MealOut{Kcal: "1200", Protein: "50", Carbs: "120", Fat: "55"}
	s.SavesMealOut(unknown)
	s.IsConfirmed("Profile saved.")
	s.Open("/profile")
	if got := mealOutOnForm(t, s.Page().Body); got != unknown {
		t.Errorf("the Meal Out estimate reopened shows %+v, want fibre and sugar unknown: %+v", got, unknown)
	}
}

// A required value missing or negative, or a value not whole, is refused on its field,
// saving nothing, with what was typed kept.
func TestARefusedMealOutValueIsRefusedOnItsField(t *testing.T) {
	cases := []struct {
		name   string
		change func(*apptest.MealOut)
		shown  string
	}{
		{"missing", func(m *apptest.MealOut) { m.Protein = "" }, "Protein (g) Required."},
		{"negative", func(m *apptest.MealOut) { m.Kcal = "-100" }, "Calories (kcal) Can't be below 0."},
		{"negative fibre", func(m *apptest.MealOut) { m.Fibre = "-1" }, "Fibre (g) Can't be below 0."},
		{"not whole", func(m *apptest.MealOut) { m.Fat = "40.5" }, "Fat (g) Type a whole number."},
		{"too large", func(m *apptest.MealOut) { m.Carbs = "10000" }, "Carbs (g) Must be below 10000."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := profileOf(t, "ada@example.com")
			typed := apptest.MealOut{Kcal: "850", Protein: "40", Carbs: "90", Fat: "30", Fibre: "5", Sugar: ""}
			c.change(&typed)
			s.SavesMealOut(typed)
			if s.Page().StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("the refused Meal Out estimate answered %d, want 422", s.Page().StatusCode)
			}
			s.Sees(c.shown)
			s.IsNotConfirmed()
			if got := mealOutOnForm(t, s.Page().Body); got != typed {
				t.Errorf("the refused Meal Out estimate holds %+v, want what was typed, %+v", got, typed)
			}
			s.Open("/profile")
			if got := mealOutOnForm(t, s.Page().Body); got != defaultMealOut {
				t.Errorf("after a refusal the Meal Out estimate is %+v, want the default %+v", got, defaultMealOut)
			}
		})
	}
}

// Posted by htmx, the Meal Out section answers with itself: saved with a Notice, or
// refused as typed.
func TestTheMealOutSectionAnswersHTMXWithItself(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	form := apptest.Form(t, s.Page().Body, "profile-meal-out")
	refusedEstimate := apptest.MealOut{Kcal: "", Protein: "40", Carbs: "90", Fat: "30"}
	refusedEstimate.Fill(form)

	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("the refused Meal Out estimate answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if got := mealOutOnForm(t, refused.Body); got != refusedEstimate {
		t.Errorf("the refused Meal Out estimate holds %+v, want %+v", got, refusedEstimate)
	}
	if shown := apptest.Text(refused.Body); !strings.Contains(shown, "Calories (kcal) Required.") {
		t.Errorf("the refused Meal Out estimate shows %q, want the calories refused", shown)
	}
	if notices := apptest.Notices(t, refused.Body); len(notices) > 0 {
		t.Errorf("the refused Meal Out estimate confirms %q", notices)
	}

	savedEstimate := apptest.MealOut{Kcal: "900", Protein: "40", Carbs: "90", Fat: "30"}
	savedEstimate.Fill(form)
	saved := s.HTMXPostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") {
		t.Fatalf("saving the Meal Out estimate answered %d, want 200 and a fragment:\n%s", saved.StatusCode, saved.Body)
	}
	if got := mealOutOnForm(t, saved.Body); got != savedEstimate {
		t.Errorf("the saved Meal Out estimate holds %+v, want %+v", got, savedEstimate)
	}
	if notices := apptest.Notices(t, saved.Body); !slices.Equal(notices, []string{"Profile saved."}) {
		t.Errorf("saving the Meal Out estimate confirms %q", notices)
	}
}

// Each User sees only their own Meal Out estimate.
func TestEachUserHasTheirOwnMealOutEstimate(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	own := apptest.MealOut{Kcal: "800", Protein: "30", Carbs: "80", Fat: "35"}
	s.SavesMealOut(own)
	s.SignsOut()

	s.Invite("bea@example.com")
	s.SignsIn("bea@example.com")
	s.ChoosesLanguage("English")
	s.Open("/profile")
	if got := mealOutOnForm(t, s.Page().Body); got != defaultMealOut {
		t.Errorf("another User's Meal Out estimate is %+v, want the default %+v", got, defaultMealOut)
	}

	s.SwitchesTo("ada@example.com")
	s.Open("/profile")
	if got := mealOutOnForm(t, s.Page().Body); got != own {
		t.Errorf("the first User's Meal Out estimate is %+v, want %+v", got, own)
	}
}
