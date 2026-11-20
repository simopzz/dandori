package app_test

import (
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// saturday is a Cycle whose Delivery comes on Friday 6 November between 10 am and noon,
// before friday ends: the Slots of Friday before noon stay with friday.
var saturday = apptest.Cycle{Delivery: "11/06/2026", From: "10am", To: "12pm", Ends: "11/07/2026"}

// monday is a Cycle whose Delivery comes on Monday 2 November between 6 and 8 pm, the
// clock's first day, ending on Wednesday 4, before friday's Delivery.
var monday = apptest.Cycle{Delivery: "11/02/2026", From: "6pm", To: "8pm", Ends: "11/04/2026"}

// With a current and an upcoming Cycle, the Cycle page opens on the current one, its
// pending Plan Proposal at the top, with the upcoming one a link away, tagged with how
// planning it stands. Dropping the upcoming one leaves the current one's pending
// Plan Proposal.
func TestTheCyclePageWithACurrentAndAnUpcomingCycle(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.At("Thu 20:00")
	s.OpensCycle(saturday)

	s.Open("/plan")
	s.Sees("Current · Thu, Nov 5 – Fri, Nov 6 Upcoming · Fri, Nov 6 – Sat, Nov 7")
	if got := apptest.CurrentLink(t, s.Page().Body); got != "Current · Thu, Nov 5 – Fri, Nov 6" {
		t.Errorf("the Cycle page shows %q, want the current Cycle", got)
	}
	s.Sees("Cycle · Thu, Nov 5 – Fri, Nov 6")
	s.Sees("01 Plan Proposal")
	s.Sees("Rice bowls for Friday.")
	s.DoesNotSee("Tentative")

	s.Follows("Upcoming · Fri, Nov 6")
	s.Sees("Cycle · Fri, Nov 6 – Sat, Nov 7")
	s.Sees("Delivery Fri, Nov 6, 10:00 am to 12:00 pm Tentative")
	s.Sees("Plan the Cycle")
	s.DoesNotSee("Rice bowls for Friday.")

	s.DropsCycle()
	s.IsConfirmed("Cycle dropped.")
	s.Sees("Cycle · Thu, Nov 5 – Fri, Nov 6")
	s.Sees("Rice bowls for Friday.")
	s.DoesNotSee("Upcoming ·")
}

// The upcoming Cycle's link says how planning it stands, so a Plan Proposal waiting, a
// planning Run going or one failed is never hidden behind the current Cycle.
func TestTheUpcomingCyclesLinkSaysHowPlanningItStands(t *testing.T) {
	cases := []struct {
		name  string
		plans func(t *testing.T, s *apptest.Scenario)
		tag   string
	}{
		{"a Plan Proposal waiting", func(t *testing.T, s *apptest.Scenario) { plansFriday(t, s, "Rice bowls for Friday.") }, "Plan Proposal waiting"},
		{"being planned", func(t *testing.T, s *apptest.Scenario) {
			s.Model.Script(fakellm.Hang())
			s.OpensCycle(friday)
			s.StartsPlanning()
		}, "Being planned"},
		{"failed", func(t *testing.T, s *apptest.Scenario) {
			s.Model.Script(fakellm.Reply("No Plan today."))
			s.OpensCycle(friday)
			s.PlansCycle()
		}, "Failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := plannerWithKitchen(t, "ada@example.com")
			s.OpensCycle(monday)
			s.At("Mon 20:00")
			c.plans(t, s)
			s.Open("/plan")
			s.Sees("Current · Mon, Nov 2 – Wed, Nov 4 Upcoming · Thu, Nov 5 – Fri, Nov 6 " + c.tag)
			s.Sees("Cycle · Mon, Nov 2 – Wed, Nov 4")
		})
	}
}

// A Delivery arriving before the current Cycle ends closes it: its pending Plan Proposal
// is rejected, naming the Delivery, and the Cycle that arrived becomes current, free to
// be planned. The beat applies it, with no page opened.
func TestAnEarlyDeliveryRejectsTheClosingCyclesPlanProposal(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.At("Thu 20:00")
	s.OpensCycle(saturday)
	s.At("Fri 12:00")
	var status, closedBy string
	s.Scan(t, &status, `SELECT status FROM plan_proposal`)
	s.Scan(t, &closedBy, `SELECT coalesce(left(ended_by->'closed_by'->>'On', 10), '') FROM plan_proposal`)
	if status != "rejected" || closedBy != "2026-11-06" {
		t.Errorf("the closing Cycle's Plan Proposal is %s, ended by the Delivery of %q, want rejected by Friday's", status, closedBy)
	}

	s.Open("/plan")
	s.Sees("Cycle · Fri, Nov 6 – Sat, Nov 7")
	s.DoesNotSee("Upcoming ·")
	s.DoesNotSee("Tentative")
	s.DoesNotSee("Rice bowls for Friday.")
	s.Model.Script(fakellm.Hang())
	s.Follows("Plan the Cycle")
	s.StartsPlanning()
	s.Sees("Planning a Cycle Going")
}

// A Delivery arriving before the current Cycle ends cancels the closing Cycle's planning
// Run, which names the Delivery. The upcoming Cycle's own Run is left going when its
// Delivery arrives.
func TestAnEarlyDeliveryCancelsTheClosingCyclesPlanningRun(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(fakellm.Hang())
	s.OpensCycle(friday)
	s.StartsPlanning()
	run := s.Page().Path
	s.At("Thu 20:00")
	s.Open(run)
	s.Sees("Planning a Cycle Going")

	s.OpensCycle(saturday)
	s.At("Fri 12:00")
	var status string
	s.Scan(t, &status, `SELECT status FROM run`)
	if status != "cancelled" {
		t.Errorf("the closing Cycle's planning Run is %s, want cancelled", status)
	}
	s.Open(run)
	s.Sees("Planning a Cycle Cancelled")
	s.Sees("The Delivery of Fri, Nov 6, 10:00 am to 12:00 pm arrived and closed its Cycle.")
}

// Once the next Cycle is opened, the current one's Plan reaches as far as its Delivery
// arriving: the delivery day's earlier Slots stay the current Cycle's, and the later
// ones are the next one's. When it arrives, what is left carries over, and the next
// Cycle is planned from its first Slot after the window.
func TestTheDeliveryDaysEarlierSlotsStayWithTheClosingCycle(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.Open("/plan")
	s.Submits("approve")
	s.At("Thu 20:00")
	s.Open("/plan")
	s.Sees("Rice bowl 1.25× · 438 kcal · 9 g protein Lunch cooked")

	s.OpensCycle(saturday)
	s.Open("/plan")
	s.Sees("Cycle · Thu, Nov 5 – Fri, Nov 6")
	s.Sees("yogurt 90 kcal · 15 g protein Breakfast in the fridge")
	s.DoesNotSee("Lunch cooked")
	s.DoesNotSee("Snack already in the kitchen")

	s.At("Fri 12:00")
	s.Open("/plan")
	s.Sees("Cycle · Fri, Nov 6 – Sat, Nov 7")
	s.Follows("Plan the Cycle")
	s.Sees("rice 800 g")
	s.Sees("lasagne × 3")
	s.Model.Script(fakellm.Hang())
	s.StartsPlanning()
	got := lastPrompt(t, s)
	for _, want := range []string{"Its Slots run from lunch on Fri 6 Nov to dinner on Sat 7 Nov.", "Portion 3: lasagne, freezer", "800 g rice (Ingredient 1), pantry"} {
		if !strings.Contains(got, want) {
			t.Errorf("the planner of the Cycle arrived isn't told %q:\n%s", want, got)
		}
	}
}

// A Cycle whose Delivery arrives with no Plan becomes current anyway, and is planned as
// current, from its first Slot not yet prepared.
func TestACycleArrivingWithNoPlanBecomesCurrentAndCanBePlanned(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.OpensCycle(thursday)
	s.At("Fri 15:00")
	s.Open("/plan")
	s.Sees("Cycle · Thu, Nov 5 – Sun, Nov 15")
	s.DoesNotSee("Tentative")
	s.DoesNotSee("Drop the Cycle")
	s.Model.Script(fakellm.Hang())
	s.Follows("Plan the Cycle")
	s.StartsPlanning()
	s.Sees("Planning a Cycle Going")
	promptHolds(t, s, "Plan every Slot from snack on Fri 6 Nov on.", "Today is Friday 6 November 2026.")
}

// Through a browser: with a current and an upcoming Cycle, the Cycle page opens on the
// current one, its pending Plan Proposal first, the two Cycles linked above its title,
// and the upcoming one's link opens it.
func TestBrowserShowsTheCyclePageWithACurrentAndAnUpcomingCycle(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.At("Thu 20:00")
	s.OpensCycle(saturday)

	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/plan")
	b.SeesInTurn(t,
		"Current · Thu, Nov 5 – Fri, Nov 6",
		"Upcoming · Fri, Nov 6 – Sat, Nov 7",
		"Cycle · Thu, Nov 5 – Fri, Nov 6",
		"01 Plan Proposal",
		"Rice bowls for Friday.",
	)
	current := b.Locator("nav.seg-links [aria-current=true]")
	if err := playwright.NewPlaywrightAssertions().Locator(current).ToHaveText("Current · Thu, Nov 5 – Fri, Nov 6"); err != nil {
		t.Errorf("the Cycle page doesn't mark the current Cycle as shown: %v", err)
	}
	b.Shot(t, "cycle-current")

	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Upcoming · Fri, Nov 6 – Sat, Nov 7"}).Click(); err != nil {
		t.Fatalf("follow the upcoming Cycle: %v", err)
	}
	b.IsAt(t, "/plan/cycles/2")
	b.SeesInTurn(t, "Cycle · Fri, Nov 6 – Sat, Nov 7", "Delivery Fri, Nov 6, 10:00 am to 12:00 pm Tentative", "Plan the Cycle")
	b.Shot(t, "cycle-upcoming-linked")
}
