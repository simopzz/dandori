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
	s.Sees("Monday 2 November")
	s.Sees("Next: breakfast at 08:00")
	s.Sees("No Cycle yet.")

	s.At("Mon 08:00")
	s.Open("/")
	s.Sees("Now: breakfast at 08:00")
	s.Sees("Later today lunch 13:00 snack 17:00 dinner 20:00")

	s.At("Mon 13:30")
	s.Open("/")
	s.Sees("Now: lunch at 13:00")
	s.Sees("Later today snack 17:00 dinner 20:00")

	s.At("Mon 21:00")
	s.Open("/")
	s.Sees("Now: dinner at 20:00")
	s.DoesNotSee("Later today")

	s.At("Mon 21:01")
	s.Open("/")
	s.Sees("Next: breakfast tomorrow at 08:00")
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
		{"Mon 09:00", "Now: breakfast at 08:00"},
		{"Mon 09:01", "Next: lunch at 13:00"},
		{"Mon 11:00", "Next: lunch at 13:00"},
		{"Mon 13:00", "Now: lunch at 13:00"},
		{"Mon 14:01", "Next: snack at 17:00"},
		{"Mon 17:30", "Now: snack at 17:00"},
		{"Mon 18:30", "Next: dinner at 20:00"},
		{"Mon 20:00", "Now: dinner at 20:00"},
		{"Mon 23:59", "Next: breakfast tomorrow at 08:00"},
		{"Tue 00:00", "Next: breakfast at 08:00"},
		{"Tue 06:00", "Next: breakfast at 08:00"},
	} {
		s.At(step.at)
		s.Open("/")
		s.Sees(step.sees)
	}
	s.Sees("Tuesday 3 November")
	s.Sees("Later today lunch 13:00 snack 17:00 dinner 20:00")
}

// Today follows the Profile's times of day, not the defaults.
func TestTodayFollowsTheProfilesTimesOfDay(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.HasTimesOfDay("ada@example.com", "07:30", "12:30", "16:00", "19:30")

	s.At("Mon 08:00")
	s.Open("/")
	s.Sees("Now: breakfast at 07:30")

	s.At("Mon 13:30")
	s.Open("/")
	s.Sees("Now: lunch at 12:30")

	s.At("Mon 21:00")
	s.Open("/")
	s.Sees("Next: breakfast tomorrow at 07:30")
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
	s.Sees("Ancora nessun Ciclo.")

	s.At("Mon 22:00")
	s.Open("/")
	s.Sees("A seguire: colazione domani alle 08:00")
}
