package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// foodsOnForm are the `exclusions` and `allergies` the form of their section on
// `page` holds.
func foodsOnForm(t *testing.T, page string) (exclusions, allergies string) {
	t.Helper()
	f := apptest.Form(t, page, "profile-foods")
	return f.Get("exclusions"), f.Get("allergies")
}

// Exclusions and allergies are saved and shown back normalised, one per line, and can
// each be cleared.
func TestExclusionsAndAllergiesAreSavedNormalisedAndCleared(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Sees("Exclusions and allergies")
	if ex, al := foodsOnForm(t, s.Page().Body); ex != "" || al != "" {
		t.Errorf("a new User's exclusions and allergies are %q and %q, want both empty", ex, al)
	}

	s.SavesFoods(" Peanuts ,\r\nshellfish; PEANUTS\r\n\r\n", "Sesame   Seeds\r\nfiocchi d’avena")
	s.IsAt("/profile")
	s.IsConfirmed("Profile saved.")
	wantEx, wantAl := "peanuts\nshellfish", "sesame seeds\nfiocchi d'avena"
	if ex, al := foodsOnForm(t, s.Page().Body); ex != wantEx || al != wantAl {
		t.Errorf("the saved exclusions and allergies show %q and %q, want %q and %q", ex, al, wantEx, wantAl)
	}

	s.SavesFoods("", "fiocchi d'avena")
	s.IsConfirmed("Profile saved.")
	s.Open("/profile")
	if ex, al := foodsOnForm(t, s.Page().Body); ex != "" || al != "fiocchi d'avena" {
		t.Errorf("with the exclusions cleared they show %q and %q, want none and fiocchi d'avena", ex, al)
	}

	s.SavesFoods("", " , ")
	s.Open("/profile")
	if ex, al := foodsOnForm(t, s.Page().Body); ex != "" || al != "" {
		t.Errorf("with both cleared they show %q and %q, want both empty", ex, al)
	}
}

// Exclusions and allergies are Food Terms: once one is written, the Language stays.
func TestTheLanguageStaysOnceAnExclusionOrAnAllergyIsWritten(t *testing.T) {
	cases := []struct{ name, exclusions, allergies string }{
		{"an exclusion", "liver", ""},
		{"an allergy", "", "peanuts"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := profileOf(t, "ada@example.com")
			s.SavesFoods(c.exclusions, c.allergies)
			s.ChangesLanguage("Italiano")
			s.IsRefused()
			s.Sees(msgLanguageStays)
			s.Open("/profile")
			s.Sees("Exclusions and allergies")
		})
	}
}

// Posted by htmx, the exclusions-and-allergies section answers with itself, normalised,
// with a Notice.
func TestTheFoodsSectionAnswersHTMXWithItself(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	form := apptest.Form(t, s.Page().Body, "profile-foods")
	form.Values.Set("exclusions", "Liver; LIVER, kidneys")
	form.Values.Set("allergies", "Peanuts")

	saved := s.HTMXPostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") {
		t.Fatalf("saving exclusions and allergies answered %d, want 200 and a fragment:\n%s", saved.StatusCode, saved.Body)
	}
	if ex, al := foodsOnForm(t, saved.Body); ex != "liver\nkidneys" || al != "peanuts" {
		t.Errorf("the saved section shows %q and %q, want normalised", ex, al)
	}
	if notices := apptest.Notices(t, saved.Body); !slices.Equal(notices, []string{"Profile saved."}) {
		t.Errorf("saving exclusions and allergies confirms %q", notices)
	}
}
