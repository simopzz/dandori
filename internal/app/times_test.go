package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// defaultTimes are a new User's times of day; savedTimes are the ones the tests save.
var (
	defaultTimes = apptest.TimesOfDay{Breakfast: "8:00 am", Lunch: "1:00 pm", Snack: "5:00 pm", Dinner: "8:00 pm"}
	savedTimes   = apptest.TimesOfDay{Breakfast: "7:30 am", Lunch: "12:30 pm", Snack: "4:00 pm", Dinner: "7:30 pm"}
)

// timesOnForm are the times of day the times-of-day form on page holds.
func timesOnForm(t *testing.T, page string) apptest.TimesOfDay {
	t.Helper()
	f := apptest.Form(t, page, "profile-times")
	return apptest.TimesOfDay{Breakfast: f.Get("breakfast"), Lunch: f.Get("lunch"), Snack: f.Get("snack"), Dinner: f.Get("dinner")}
}

// A new User's Profile shows the default times of day, as their Language writes them.
func TestANewUserHasTheDefaultTimesOfDay(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Sees("Times of day")
	if got := timesOnForm(t, s.Page().Body); got != defaultTimes {
		t.Errorf("a new User's times of day are %+v, want %+v", got, defaultTimes)
	}
}

// Saved times of day, typed in either form, are shown back as the Language writes them,
// and Today's next Slot follows them.
func TestTimesOfDayAreSavedAndTodayFollowsThem(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.SavesTimesOfDay(apptest.TimesOfDay{Breakfast: "07:30", Lunch: "12.30", Snack: "4pm", Dinner: "1930"})
	s.IsAt("/profile")
	s.IsConfirmed("Profile saved.")
	if got := timesOnForm(t, s.Page().Body); got != savedTimes {
		t.Errorf("the saved times of day show %+v, want %+v", got, savedTimes)
	}
	s.Open("/profile")
	if got := timesOnForm(t, s.Page().Body); got != savedTimes {
		t.Errorf("the times of day reopened show %+v, want %+v", got, savedTimes)
	}

	s.At("Mon 07:00")
	s.Open("/")
	s.Sees("Next: breakfast at 7:30 am")
	s.At("Mon 14:00")
	s.Open("/")
	s.Sees("Next: snack at 4:00 pm")
}

// A time of day not after the one before it is refused on its field, saving nothing, with
// what was typed kept.
func TestATimeOfDayNotAfterTheOneBeforeIsRefusedOnItsField(t *testing.T) {
	cases := []struct {
		name   string
		change func(*apptest.TimesOfDay)
		shown  string
	}{
		{"lunch at breakfast's time", func(ts *apptest.TimesOfDay) { ts.Lunch = "7:30 am" }, "Lunch Must be after breakfast."},
		{"dinner before snack", func(ts *apptest.TimesOfDay) { ts.Dinner = "3:00 pm" }, "Dinner Must be after snack."},
		{"not a time", func(ts *apptest.TimesOfDay) { ts.Snack = "teatime" }, "Snack Type a time, like 1:00 pm or 13:00."},
		{"missing", func(ts *apptest.TimesOfDay) { ts.Breakfast = "" }, "Breakfast Required."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := profileOf(t, "ada@example.com")
			typed := savedTimes
			c.change(&typed)
			s.SavesTimesOfDay(typed)
			if s.Page().StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("the refused times of day answered %d, want 422", s.Page().StatusCode)
			}
			s.Sees(c.shown)
			s.IsNotConfirmed()
			if got := timesOnForm(t, s.Page().Body); got != typed {
				t.Errorf("the refused times of day hold %+v, want what was typed, %+v", got, typed)
			}
			s.Open("/profile")
			if got := timesOnForm(t, s.Page().Body); got != defaultTimes {
				t.Errorf("after a refusal the times of day are %+v, want the defaults %+v", got, defaultTimes)
			}
		})
	}
}

// Posted by htmx, the times-of-day section answers with itself: saved with a Notice, or
// refused as typed.
func TestTheTimesOfDaySectionAnswersHTMXWithItself(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	form := apptest.Form(t, s.Page().Body, "profile-times")
	refusedTimes := savedTimes
	refusedTimes.Snack = "11:00 am"
	refusedTimes.Fill(form)

	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("the refused times of day answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if got := timesOnForm(t, refused.Body); got != refusedTimes {
		t.Errorf("the refused times of day hold %+v, want %+v", got, refusedTimes)
	}
	if shown := apptest.Text(refused.Body); !strings.Contains(shown, "Must be after lunch.") {
		t.Errorf("the refused times of day show %q, want the snack refused", shown)
	}
	if notices := apptest.Notices(t, refused.Body); len(notices) > 0 {
		t.Errorf("the refused times of day confirm %q", notices)
	}

	savedTimes.Fill(form)
	saved := s.HTMXPostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") {
		t.Fatalf("saving the times of day answered %d, want 200 and a fragment:\n%s", saved.StatusCode, saved.Body)
	}
	if got := timesOnForm(t, saved.Body); got != savedTimes {
		t.Errorf("the saved times of day hold %+v, want %+v", got, savedTimes)
	}
	if notices := apptest.Notices(t, saved.Body); !slices.Equal(notices, []string{"Profile saved."}) {
		t.Errorf("saving the times of day confirms %q", notices)
	}
}
