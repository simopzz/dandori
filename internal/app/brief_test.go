package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// briefOnForm is the Planning Brief the Planning Brief's form on `page` holds.
func briefOnForm(t *testing.T, page string) string {
	t.Helper()
	return apptest.Form(t, page, "profile-brief").Get("brief")
}

// A new User's Planning Brief is empty, its example in their Language just a placeholder.
func TestANewUsersPlanningBriefIsEmptyWithItsPlaceholder(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Sees("Planning Brief")
	if got := briefOnForm(t, s.Page().Body); got != "" {
		t.Errorf("a new User's Planning Brief is %q, want empty", got)
	}
	if !strings.Contains(s.Page().Body, `placeholder="Low-care Recipes, about 15 active minutes"`) {
		t.Errorf("the Planning Brief has no English placeholder:\n%s", s.Page().Body)
	}

	s.ChangesLanguage("Italiano")
	if !strings.Contains(s.Page().Body, `placeholder="Ricette che chiedono poca cura, circa 15 minuti attivi"`) {
		t.Errorf("the Planning Brief has no Italian placeholder:\n%s", s.Page().Body)
	}
}

// The Planning Brief is saved and shown back, and a blank one is saved empty.
func TestThePlanningBriefIsSavedShownBackAndSavedEmpty(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.SavesBrief("Low-care Recipes.\r\nFish twice a week.")
	s.IsAt("/profile")
	s.IsConfirmed("Profile saved.")
	if got, want := briefOnForm(t, s.Page().Body), "Low-care Recipes.\nFish twice a week."; got != want {
		t.Errorf("the saved Planning Brief shows %q, want %q", got, want)
	}

	s.SavesBrief("  \r\n ")
	s.IsConfirmed("Profile saved.")
	s.Open("/profile")
	if got := briefOnForm(t, s.Page().Body); got != "" {
		t.Errorf("the Planning Brief saved blank shows %q, want empty", got)
	}
}

// The Planning Brief isn't a Food Term: writing one leaves the Language free to change.
func TestAPlanningBriefLeavesTheLanguageFree(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.SavesBrief("Fish twice a week.")
	s.ChangesLanguage("Italiano")
	s.IsAt("/profile")
	s.IsConfirmed("Profilo salvato.")
}

// Posted by htmx, the Planning Brief's section answers with itself, with a Notice.
func TestTheBriefSectionAnswersHTMXWithItself(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	form := apptest.Form(t, s.Page().Body, "profile-brief")
	form.Values.Set("brief", "Fish twice a week.")

	saved := s.HTMXPostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") {
		t.Fatalf("saving the Planning Brief answered %d, want 200 and a fragment:\n%s", saved.StatusCode, saved.Body)
	}
	if got := briefOnForm(t, saved.Body); got != "Fish twice a week." {
		t.Errorf("the saved Planning Brief shows %q", got)
	}
	if notices := apptest.Notices(t, saved.Body); !slices.Equal(notices, []string{"Profile saved."}) {
		t.Errorf("saving the Planning Brief confirms %q", notices)
	}
}
