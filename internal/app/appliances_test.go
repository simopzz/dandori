package app_test

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// appliancesOnForm are the kinds of appliance the form of their section on `page` has
// ticked, by code, each with its note.
func appliancesOnForm(t *testing.T, page string) map[string]string {
	t.Helper()
	f := apptest.Form(t, page, "profile-appliances")
	owned := map[string]string{}
	for _, code := range f.Values["appliances"] {
		owned[code] = f.Get("note-" + code)
	}
	return owned
}

// A new User owns a hob and an oven, and nothing else, until they say otherwise.
func TestANewUserOwnsAHobAndAnOven(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Sees("Appliances")
	if got, want := appliancesOnForm(t, s.Page().Body), map[string]string{"oven": "", "hob": ""}; !maps.Equal(got, want) {
		t.Errorf("a new User owns %q, want %q", got, want)
	}
}

// The kinds of appliance show in the User's Language, in the list's order.
func TestTheKindsOfApplianceShowInTheUsersLanguage(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.ChangesLanguage("Italiano")
	s.Open("/profile")
	s.Sees("Elettrodomestici")
	text := apptest.Text(s.Page().Body)
	if forno, piano, vaporiera := strings.Index(text, "Forno"), strings.Index(text, "Piano cottura"), strings.Index(text, "Vaporiera"); forno < 0 || forno > piano || piano > vaporiera {
		t.Errorf("the kinds of appliance don't show in Italian in the list's order:\n%s", text)
	}
}

// Kinds ticked and unticked are saved with their notes and shown back, and a note on a
// kind left unticked is dropped.
func TestAppliancesAreSavedWithTheirNotesAndAnUntickedKindsNoteDropped(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.SavesAppliances([]string{"hob", "pressure_cooker", "air_fryer"}, map[string]string{
		"pressure_cooker": " Instant Pot Duo 6 L ", "hob": "induction", "oven": "Bosch, broken",
	})
	s.IsAt("/profile")
	s.IsConfirmed("Profile saved.")
	want := map[string]string{"hob": "induction", "pressure_cooker": "Instant Pot Duo 6 L", "air_fryer": ""}
	if got := appliancesOnForm(t, s.Page().Body); !maps.Equal(got, want) {
		t.Errorf("the saved appliances show %q, want %q", got, want)
	}
	if note := apptest.Form(t, s.Page().Body, "profile-appliances").Get("note-oven"); note != "" {
		t.Errorf("the unticked oven kept its note %q", note)
	}

	s.SavesAppliances(nil, map[string]string{"hob": "induction"})
	s.IsConfirmed("Profile saved.")
	s.Open("/profile")
	if got := appliancesOnForm(t, s.Page().Body); len(got) != 0 {
		t.Errorf("with every kind unticked the User owns %q, want none", got)
	}
	if note := apptest.Form(t, s.Page().Body, "profile-appliances").Get("note-hob"); note != "" {
		t.Errorf("the unticked hob kept its note %q", note)
	}
}

// An appliance's note isn't a Food Term: writing one leaves the Language free to change.
func TestAnAppliancesNoteLeavesTheLanguageFree(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.SavesAppliances([]string{"pressure_cooker"}, map[string]string{"pressure_cooker": "Instant Pot Duo 6 L"})
	s.ChangesLanguage("Italiano")
	s.IsAt("/profile")
	s.IsConfirmed("Profilo salvato.")
}

// Posted by htmx, the appliances' section answers with itself, as saved, with a Notice.
func TestTheAppliancesSectionAnswersHTMXWithItself(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	form := apptest.Form(t, s.Page().Body, "profile-appliances")
	form.Values["appliances"] = []string{"oven", "microwave"}
	form.Values.Set("note-microwave", "Samsung")
	form.Values.Set("note-hob", "gas")

	saved := s.HTMXPostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") {
		t.Fatalf("saving appliances answered %d, want 200 and a fragment:\n%s", saved.StatusCode, saved.Body)
	}
	if got, want := appliancesOnForm(t, saved.Body), map[string]string{"oven": "", "microwave": "Samsung"}; !maps.Equal(got, want) {
		t.Errorf("the saved section shows %q, want %q", got, want)
	}
	if notices := apptest.Notices(t, saved.Body); !slices.Equal(notices, []string{"Profile saved."}) {
		t.Errorf("saving appliances confirms %q", notices)
	}
}
