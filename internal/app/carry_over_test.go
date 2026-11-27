package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// afterFriday is a Cycle whose Delivery comes on Friday 6 November between 9 and 10 pm,
// after friday's dinner, ending on Sunday 8: its first Slot is Saturday's breakfast.
var afterFriday = apptest.Cycle{Delivery: "11/06/2026", From: "9pm", To: "10pm", Ends: "11/08/2026"}

// Planning the upcoming Cycle, the planner is told what the kitchen will hold at its
// first Slot: what it holds now, less what the current Plan's Prep Sessions use up and
// the Portions without a Slot it eats, with the Portions it prepares and leaves without a
// Slot, by PlannedID.
func TestPlanningTheUpcomingCycleIsToldWhatCarriesOver(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.Open("/plan")
	s.Submits("approve")
	s.At("Thu 20:00")

	s.Model.Script(fakellm.Hang())
	s.OpensCycle(afterFriday)
	s.StartsPlanning()
	told(t, s, []string{
		"What the kitchen will hold at breakfast on Sat 7 Nov, the first Slot to plan",
		"- 425 g rice (Ingredient 1), pantry",
	}, "800 g rice", "don't put them in Slots")
	if n := strings.Count(lastPrompt(t, s), ": Rice bowl, freezer"); n != 1 {
		t.Errorf("the planner is told %d Portions of Rice bowl in the freezer, want the 1 Friday's lunch freezes", n)
	}
	if n := strings.Count(lastPrompt(t, s), ": lasagne, freezer"); n != 2 {
		t.Errorf("the planner is told %d Portions of lasagne, want the 2 Friday's snack leaves", n)
	}
}

// The upcoming Cycle's Plan puts the forecast carry-over in Slots before it is prepared:
// planned on Thursday, Saturday's dinner eats the Rice bowl Friday's lunch freezes, which
// Friday evening thaws, and the Plan shows it there.
func TestTheUpcomingPlanPutsAPortionNotYetPreparedInASlot(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Thu 20:00")
	approves(t, s, func(req agent.Request) map[string]any {
		return map[string]any{
			"prep_sessions": yogurtAt(idIn(t, req, `Ingredient (\d+): yogurt`), []string{"2026-11-07", "2026-11-08"}, "2026-11-07 dinner"),
			"kept":          []map[string]any{{"portion_id": idIn(t, req, `Portion (-\d+): Rice bowl, freezer`), "on": "2026-11-07", "kind": "dinner"}},
		}
	}, func() { s.OpensCycle(afterFriday); s.PlansCycle() })
	s.OpensUpcomingCycle()
	s.Sees("Rice bowl 438 kcal · 9 g protein Dinner already in the kitchen")

	s.At("Sat 13:05")
	s.Open("/kitchen")
	s.Sees("Count Rice bowl in the fridge")
	s.At("Sat 20:05")
	s.Open("/kitchen")
	s.DoesNotSee("Rice bowl")
}

// The upcoming Cycle waits its turn: while the current Cycle's Plan Proposal is pending,
// planning it is refused, and once that is decided it can be planned.
func TestPlanningTheUpcomingCycleWaitsForTheCurrentPlanProposal(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.At("Thu 20:00")

	s.OpensCycle(afterFriday)
	check := s.Page().Path
	s.StartsPlanning()
	if s.Page().StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("planning the upcoming Cycle while the current one's Plan Proposal is pending = %d, want %d", s.Page().StatusCode, http.StatusUnprocessableEntity)
	}
	s.Sees("Another planning Run is going, or a Plan Proposal is waiting for you.")

	s.Open("/plan")
	s.Submits("approve")
	s.Model.Script(fakellm.Hang())
	s.Open(check)
	s.StartsPlanning()
	s.Sees("Planning a Cycle Going")
}
