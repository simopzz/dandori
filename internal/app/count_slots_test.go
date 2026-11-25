package app_test

import (
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// yogurtAt is Prep Sessions of 150 g of yogurt `pot` set out at each Slot of `days`
// except those in `except`, written as "2026-11-07 lunch", each eaten at its Slot.
func yogurtAt(pot int64, days []string, except ...string) []map[string]any {
	var sessions []map[string]any
	for _, day := range days {
		for _, kind := range []string{"breakfast", "lunch", "snack", "dinner"} {
			if slices.Contains(except, day+" "+kind) {
				continue
			}
			sessions = append(sessions, map[string]any{"on": day, "kind": kind, "ingredient_id": pot, "quantity": 150, "portions": []map[string]any{
				{"location": "fridge", "eat_on": day, "eat_kind": kind},
			}})
		}
	}
	return sessions
}

// approves scripts a planning Run into a Plan Proposal of what `read` reads from the
// prompt, which the step `plan` starts, and approves it on its Cycle's page: the
// upcoming Cycle's, or the Plan page's with none.
func approves(t *testing.T, s *apptest.Scenario, read func(agent.Request) map[string]any, plan func()) {
	t.Helper()
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", read(req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("A Plan."),
	)
	plan()
	s.OpensUpcomingCycle()
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
}

// Counting a Portion Lot down keeps the Portion the Plan puts in a Slot: of the three
// lasagne, Friday's snack is filled by one, so counting down to one keeps it, and the
// snack eats it.
func TestCountingPortionsDownKeepsTheOneFillingASlot(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.CountsPortions(countLasagne, "1", "")
	s.IsConfirmed("Counted lasagne: 1 in the freezer.")
	s.At("Fri 17:05")
	s.Open("/kitchen")
	s.DoesNotSee("lasagne")
}

// Among Portions that each fill a Slot, counting down takes the one with the latest:
// with the three lasagne at Friday's lunch, snack and dinner, counting down to two
// takes dinner's, so by the snack none is left.
func TestCountingPortionsDownTakesTheLatestSlot(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approves(t, s, func(req agent.Request) map[string]any {
		lasagne := idsIn(t, req, `Portion (\d+): lasagne`)
		var kept []map[string]any
		for i, kind := range []string{"lunch", "snack", "dinner"} {
			kept = append(kept, map[string]any{"portion_id": lasagne[i], "on": "2026-11-06", "kind": kind})
		}
		return map[string]any{
			"prep_sessions": yogurtAt(idIn(t, req, `Ingredient (\d+): yogurt`), []string{"2026-11-06"}, "2026-11-06 lunch", "2026-11-06 snack", "2026-11-06 dinner"),
			"kept":          kept,
		}
	}, func() { s.OpensCycle(friday); s.PlansCycle() })

	s.CountsPortions(countLasagne, "2", "")
	s.IsConfirmed("Counted lasagne: 2 in the freezer.")
	s.At("Fri 17:05")
	s.Open("/kitchen")
	s.DoesNotSee("lasagne")
}

// A Portion the upcoming Cycle's Plan puts in a Slot has one: on Friday evening, with
// friday current, the two lasagne its snack left are counted down to one, which keeps
// the one Saturday's lunch eats.
func TestCountingPortionsDownKeepsTheOneTheUpcomingPlanPlaces(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Thu 20:00")
	approves(t, s, func(req agent.Request) map[string]any {
		return map[string]any{
			"prep_sessions": yogurtAt(idIn(t, req, `Ingredient (\d+): yogurt`), []string{"2026-11-07", "2026-11-08"}, "2026-11-07 lunch"),
			"kept":          []map[string]any{{"portion_id": idIn(t, req, `Portion (\d+): lasagne`), "on": "2026-11-07", "kind": "lunch"}},
		}
	}, func() { s.OpensCycle(afterFriday); s.PlansCycle() })

	s.At("Fri 18:00")
	s.CountsPortions(countLasagne, "1", "")
	s.IsConfirmed("Counted lasagne: 1 in the freezer.")
	s.At("Sat 13:05")
	s.Open("/kitchen")
	s.DoesNotSee("lasagne")
}

// A Planned Portion a replan moves to a later Slot is eaten there, not at the Slot first
// planned: the Rice bowl Friday's dinner prepared for Sunday's lunch, moved on Saturday
// to Sunday's dinner, is still in the fridge after lunch and eaten at dinner.
func TestAPortionAReplanMovesIsEatenAtItsNewSlot(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approves(t, s, func(req agent.Request) map[string]any { return weekendDraft(t, req) }, func() { s.OpensCycle(weekend); s.PlansCycle() })

	s.At("Sat 10:00")
	s.Open("/plan")
	approves(t, s, func(req agent.Request) map[string]any {
		sessions := yogurtAt(idIn(t, req, `Ingredient (\d+): yogurt`), []string{"2026-11-07", "2026-11-08"}, "2026-11-07 breakfast", "2026-11-08 dinner")
		return map[string]any{
			"prep_sessions": sessions,
			"kept":          []map[string]any{{"portion_id": idIn(t, req, `Portion (-\d+): Rice bowl, fridge`), "on": "2026-11-08", "kind": "dinner"}},
		}
	}, func() { s.AsksForChanges("The Rice bowl at Sunday's dinner."); s.Settle() })

	s.At("Sun 13:05")
	s.Open("/kitchen")
	s.Sees("Count Rice bowl in the fridge")
	s.At("Sun 20:05")
	s.Open("/kitchen")
	s.DoesNotSee("Count Rice bowl in the fridge")
	s.Sees("Count Rice bowl in the freezer")
}
