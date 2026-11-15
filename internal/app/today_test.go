package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// Today shows the day's Slot now, or the next one, by the Profile's times of day: the
// defaults until the User changes them. The day's later Slots are a click away.
func TestTodayShowsTheSlotNowOrNext(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")

	s.At("Mon 07:00")
	s.Open("/")
	s.Sees("Monday, November 2")
	s.Sees("Next: breakfast at 8:00 am")

	s.At("Mon 08:00")
	s.Open("/")
	s.Sees("Now: breakfast at 8:00 am")
	s.Sees("Later today lunch 1:00 pm snack 5:00 pm dinner 8:00 pm")

	s.At("Mon 13:30")
	s.Open("/")
	s.Sees("Now: lunch at 1:00 pm")
	s.Sees("Later today snack 5:00 pm dinner 8:00 pm")

	s.At("Mon 21:00")
	s.Open("/")
	s.Sees("Now: dinner at 8:00 pm")
	s.DoesNotSee("Later today")

	s.At("Mon 21:01")
	s.Open("/")
	s.Sees("Next: breakfast tomorrow at 8:00 am")
}

// Opened through a whole day, Today moves from each Slot to the next: each is now for an
// hour after its time of day, its last minute included, and the next one is next until
// then. After midnight, breakfast is next today again.
func TestTodayThroughADay(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")

	for _, step := range []struct{ at, sees string }{
		{"Mon 09:00", "Now: breakfast at 8:00 am"},
		{"Mon 09:01", "Next: lunch at 1:00 pm"},
		{"Mon 11:00", "Next: lunch at 1:00 pm"},
		{"Mon 13:00", "Now: lunch at 1:00 pm"},
		{"Mon 14:01", "Next: snack at 5:00 pm"},
		{"Mon 17:30", "Now: snack at 5:00 pm"},
		{"Mon 18:30", "Next: dinner at 8:00 pm"},
		{"Mon 20:00", "Now: dinner at 8:00 pm"},
		{"Mon 23:59", "Next: breakfast tomorrow at 8:00 am"},
		{"Tue 00:00", "Next: breakfast at 8:00 am"},
		{"Tue 06:00", "Next: breakfast at 8:00 am"},
	} {
		s.At(step.at)
		s.Open("/")
		s.Sees(step.sees)
	}
	s.Sees("Tuesday, November 3")
	s.Sees("Later today lunch 1:00 pm snack 5:00 pm dinner 8:00 pm")
}

// Today follows the Profile's times of day, not the defaults.
func TestTodayFollowsTheProfilesTimesOfDay(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/profile")
	s.SavesTimesOfDay(apptest.TimesOfDay{Breakfast: "07:30", Lunch: "12:30", Snack: "16:00", Dinner: "19:30"})

	s.At("Mon 08:00")
	s.Open("/")
	s.Sees("Now: breakfast at 7:30 am")

	s.At("Mon 13:30")
	s.Open("/")
	s.Sees("Now: lunch at 12:30 pm")

	s.At("Mon 21:00")
	s.Open("/")
	s.Sees("Next: breakfast tomorrow at 7:30 am")
}

// Today speaks the User's Language, Slot kinds included.
func TestTodaySpeaksTheUsersLanguage(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")

	s.At("Mon 13:30")
	s.Open("/")
	s.Sees("Ora: pranzo alle 13:00")
	s.Sees("Più tardi oggi spuntino 17:00 cena 20:00")
	s.Sees("Il tuo Piano compare qui quando Dandori pianifica cosa mangi. Per pianificarlo, deve conoscere la tua cucina, le tue Ricette e i tuoi Obiettivi. Conta la tua cucina Aggiungi Ricette Imposta i tuoi Obiettivi")

	s.At("Mon 22:00")
	s.Open("/")
	s.Sees("A seguire: colazione domani alle 08:00")
}

// Until Dandori plans, a new User's Today says their Plan shows there once it does, and
// links each thing planning needs, named for what the User does there.
func TestANewUsersTodaySaysWhatPlanningNeedsAndLinksIt(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")

	s.Open("/")
	s.Sees("Your Plan shows here once Dandori plans what you eat. To plan it, it needs to know your kitchen, your Recipes and your Targets. Count your kitchen Add Recipes Set your Targets")
	for _, link := range []struct{ text, at string }{
		{"Count your kitchen", "/kitchen/count"},
		{"Add Recipes", "/recipes"},
		{"Set your Targets", "/profile"},
	} {
		s.Open("/")
		s.Follows(link.text)
		s.IsAt(link.at)
	}
}
