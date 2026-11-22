package app_test

import (
	"cmp"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent/fakellm"
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

// From three days before the current Cycle's end date, not before, Today says to open
// the next Cycle, linking to the form opening one, until it is open.
func TestTodaySaysToOpenTheNextCycleFromThreeDaysBeforeItsEnd(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(thursday) // ending on Sunday 15 November
	s.At("Thu 20:00")
	s.Open("/")
	s.DoesNotSee("Open the next Cycle")
	s.At("2026-11-11 23:59")
	s.Open("/")
	s.DoesNotSee("Open the next Cycle")

	s.At("2026-11-12 00:00")
	s.Open("/")
	s.Follows("Open the next Cycle")
	s.IsAt("/plan/open")
	s.OpensCycle(apptest.Cycle{Delivery: "11/15/2026", From: "6pm", To: "8pm"})
	s.IsConfirmed("Cycle opened.")
	s.Open("/")
	s.DoesNotSee("Open the next Cycle")
}

// Once the next Cycle is open, Today says how planning it stands, linking to the Cycle
// page: waiting to be planned, being planned, failed, or a Plan Proposal waiting,
// straight to the Plan Proposal.
func TestTodaySaysHowPlanningTheNextCycleStands(t *testing.T) {
	cases := []struct {
		name  string
		plans func(t *testing.T, s *apptest.Scenario)
		// sees is the row's text, link its link's when not all of it, and at where
		// it leads.
		sees, link, at string
	}{
		{"waiting to be planned", func(t *testing.T, s *apptest.Scenario) { s.OpensCycle(friday) }, "The next Cycle is waiting to be planned", "", "/plan/cycles/2"},
		{"being planned", func(t *testing.T, s *apptest.Scenario) {
			s.Model.Script(fakellm.Hang())
			s.OpensCycle(friday)
			s.StartsPlanning()
		}, "The next Cycle is being planned", "", "/plan/cycles/2"},
		{"failed", func(t *testing.T, s *apptest.Scenario) {
			s.Model.Script(fakellm.Reply("No Plan today."))
			s.OpensCycle(friday)
			s.PlansCycle()
		}, "Planning the next Cycle Failed", "Planning the next Cycle", "/plan/cycles/2"},
		{"a Plan Proposal waiting", func(t *testing.T, s *apptest.Scenario) { plansFriday(t, s, "Rice bowls for Friday.") }, "A Plan Proposal for the next Cycle is waiting", "", "/plan/cycles/2#plan-proposal"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := plannerWithKitchen(t, "ada@example.com")
			s.OpensCycle(monday)
			s.At("Mon 20:00")
			c.plans(t, s)
			s.Open("/")
			s.Sees(c.sees)
			s.DoesNotSee("Open the next Cycle")
			link := cmp.Or(c.link, c.sees)
			if got := apptest.Link(t, s.Page().Body, link); got != c.at {
				t.Errorf("%q leads to %q, want %q", link, got, c.at)
			}
			s.Follows(link)
			s.Sees("Cycle · Thu, Nov 5 – Fri, Nov 6")
			if _, anchor, ok := strings.Cut(c.at, "#"); ok {
				if got := apptest.Anchored(t, s.Page().Body, anchor); !strings.HasPrefix(got, "01 Plan Proposal Rice bowls for Friday.") {
					t.Errorf("%q lands on %q, want Section 01, the Plan Proposal", link, got)
				}
			}
		})
	}
}

// Once the next Cycle has a Plan, with nothing pending, Today says nothing of it.
func TestTodaySaysNothingOfAPlannedNextCycle(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.OpensCycle(monday)
	s.At("Mon 20:00")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.OpensUpcomingCycle()
	s.Submits("approve")
	s.Open("/")
	s.DoesNotSee("next Cycle")
}

// A Cycle whose Delivery arrived without a Plan is current, and Today says to plan it,
// linking to its Cycle page, then that it is being planned.
func TestTodaySaysHowPlanningTheCurrentCycleStands(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.OpensCycle(friday)
	s.At("Thu 20:00")
	s.Open("/")
	s.Follows("The current Cycle is waiting to be planned")
	s.IsAt("/plan/cycles/1")

	s.Model.Script(fakellm.Hang())
	s.Follows("Plan the Cycle")
	s.StartsPlanning()
	s.Open("/")
	s.Sees("The current Cycle is being planned")
	s.DoesNotSee("waiting to be planned")
}

// While the Targets aren't set, Today points to the Profile instead of offering to open
// a Cycle, which would be refused; once they are, it offers to.
func TestTodayPointsToTheProfileUntilTheTargetsAreSet(t *testing.T) {
	s := profileOf(t, "ada@example.com")
	s.Open("/")
	s.DoesNotSee("Open a Cycle")
	if got := apptest.Link(t, s.Page().Body, "Planning needs your Targets."); got != "/profile#targets" {
		t.Errorf("Today's pointer leads to %q, want /profile#targets", got)
	}

	s.Open("/profile")
	s.SavesTargets(targets)
	s.Open("/")
	s.DoesNotSee("Planning needs your Targets.")
	s.Sees("Open a Cycle")
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
