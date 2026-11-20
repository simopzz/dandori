package app_test

import (
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// Friday under fridayDraft eats yogurt (90 kcal, 15 g protein, its fibre and sugar
// unknown), two Rice bowls at 1.25× (438 kcal, 9 g protein, nothing known of their fibre
// or sugar) and lasagne (620 kcal, 32 g protein, 4 g fibre, 8 g sugar): 1586 kcal, 65 g
// protein, at least 4 g fibre and at least 8 g sugar, against 2024–2376 kcal, at least
// 130 g protein and 30 g fibre, at most 50 g sugar.

// A Plan submitted is stored with its soft misses. The Plan Proposal shows first the
// average under heads naming each Target's unit and bounds, each miss marked under or
// over and a floor written "4+", then the soft misses grouped by kind with that
// information; then its days under the same heads: the seven days before it, those no
// Plan covered as one range, and each planned day with its misses marked, what it cooks,
// its Slots and the Prep Sessions it holds.
func TestAPlanProposalShowsItsSoftMissesAndItsDays(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")

	s.Open("/plan")
	s.Sees("01 Plan Proposal Friday. Average " + heads +
		" calories 1586 under protein 65 under fibre at least 4 + sugar at least 8 + A day is marked")
	s.Sees("1 day more than 25% off Fri, Nov 6: calories under protein under " +
		"Rice bowl eaten back to back, Fri, Nov 6 " +
		"lasagne: 2 of 3 left unused, in the freezer Rice bowl: 1 made for after the Cycle, in the freezer Approve")
	s.Sees("02 Days " + heads + " 7 days before 7 with no Plan Fri, Oct 30 – Thu, Nov 5: no Plan " +
		"Friday, November 6 calories 1586 under protein 65 under fibre at least 4 + sugar at least 8 + Rice bowl · 10 min active")
	s.Sees("Slots yogurt 90 kcal · 15 g protein Breakfast in the fridge")
	s.Sees("Prep Sessions Rice bowl 3 at 1.25× 25 min · 10 active Ingredients rice 375 g")
}

// heads are the four Targets' heads under targets: each names its Target's unit, then
// its bounds.
const heads = "Calories, kcal 2024–2376 Protein, g ≥ at least 130 Fibre, g ≥ at least 30 Sugar, g ≤ at most 50"

// Changing the Targets once a Plan Proposal is made leaves what it shows unchanged: it
// keeps the Targets it aimed at.
func TestChangingTheTargetsLeavesAPlanProposalUnchanged(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")
	s.Open("/profile")
	s.SavesTargets(apptest.Targets{Kcal: "1500", Protein: "60", Fibre: "3", Sugar: "100", Tolerance: "10"})

	s.Open("/plan")
	s.Sees("Average " + heads + " calories 1586 under protein 65 under")
	s.Sees("02 Days " + heads)
}

// The seven days before a Plan Proposal show what an approved Plan put in them, and the
// planner is told them.
func TestTheSevenDaysBeforeAPlanProposalShowTheApprovedPlan(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")
	s.Open("/plan")
	s.Submits("approve")
	s.At("Thu 21:00")

	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			eaten := func(kind string) map[string]any {
				return map[string]any{"location": "fridge", "eat_on": "2026-11-07", "eat_kind": kind}
			}
			return fakellm.CallTool("check_plan", map[string]any{"prep_sessions": []map[string]any{{
				"on": "2026-11-07", "kind": "breakfast", "ingredient_id": idIn(t, req, `Ingredient (\d+): yogurt`), "quantity": 150,
				"portions": []map[string]any{eaten("breakfast"), eaten("lunch"), eaten("snack"), eaten("dinner")},
			}}})
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Saturday."),
	)
	s.OpensCycle(apptest.Cycle{Delivery: "11/06/2026", From: "6pm", To: "8pm", Ends: "11/07/2026"})
	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")
	reqs := s.Model.Requests()
	if p := firstTurn(reqs[len(reqs)-3]); !strings.Contains(p, "- Fri 6 Nov: 1586 kcal, 65 g protein, at least 4 g fibre, at least 8 g sugar") ||
		!strings.Contains(p, "- Thu 5 Nov: no Plan") {
		t.Errorf("the planner isn't told the seven days before:\n%s", p)
	}

	s.OpensUpcomingCycle()
	s.Sees("7 days before 1 on the current Plan, 6 with no Plan Sat, Oct 31 – Thu, Nov 5: no Plan " +
		"Fri, Nov 6 calories 1586 protein 65 fibre at least 4 + sugar at least 8 + Saturday, November 7 calories 360")
}

// A day a little under a Target is no soft miss, so its row leaves it unmarked; the
// average missing it is marked. Friday's 65 g of protein is 7% under 70 g.
func TestADayALittleUnderATargetIsUnmarked(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Open("/profile")
	s.SavesTargets(apptest.Targets{Kcal: "1600", Protein: "70", Fibre: "30", Sugar: "50", Tolerance: "5"})
	plansFriday(t, s, "Friday.")

	s.Open("/plan")
	s.Sees("calories 1586 protein 65 under fibre at least 4 + sugar at least 8 + A day is marked")
	s.DoesNotSee("more than 25% off Fri")
	s.Sees("Friday, November 6 calories 1586 protein 65 fibre at least 4 + sugar at least 8 +")
}

// halvesTheRiceBowl edits the Rice bowl to make 4 Portions of its 200 g of rice, not 2:
// 175 kcal and 3.5 g protein a Portion, 219 kcal at 1.25×.
func halvesTheRiceBowl(s *apptest.Scenario) {
	s.Open("/recipes")
	s.Follows("Rice bowl")
	s.Follows("Edit")
	r := riceBowl()
	r.Portions = "4"
	s.PostsRecipeForm(r)
	s.IsConfirmed("Saved Rice bowl.")
}

// A pending Plan Proposal keeps the days and soft misses it was judged with when a Recipe
// it prepares is edited: they are what the User weighs approving it.
func TestAPendingPlanProposalKeepsItsVerdictWhenARecipeIsEdited(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")
	halvesTheRiceBowl(s)

	s.Open("/plan")
	s.Sees("Average " + heads + " calories 1586 under protein 65 under")
	s.Sees("1 day more than 25% off Fri, Nov 6: calories under protein under")
	s.Sees("Friday, November 6 calories 1586 under protein 65 under")
}

// An approved Plan's days follow an edit to a Recipe it has yet to prepare: editing a
// Recipe changes only what is yet to be recorded.
func TestAnApprovedPlansDaysFollowAnEditedRecipe(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")
	s.Open("/plan")
	s.Submits("approve")
	halvesTheRiceBowl(s)

	s.Open("/plan")
	s.Sees("Friday, November 6 calories 1148 under protein 55 under")
}

// A Plan whose first day another approved Plan fills from breakfast, it from lunch on,
// counts that breakfast in its first day, pending and approved alike: Friday is the
// Friday Plan's yogurt and three Portions of yogurt, 360 kcal.
func TestAPartlyPlannedFirstDayReadsTheSamePendingAndApproved(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")
	s.Open("/plan")
	s.Submits("approve")
	s.At("Thu 21:00")

	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			var portions []map[string]any
			for _, slot := range [][2]string{{"2026-11-06", "lunch"}, {"2026-11-06", "snack"}, {"2026-11-06", "dinner"},
				{"2026-11-07", "breakfast"}, {"2026-11-07", "lunch"}, {"2026-11-07", "snack"}, {"2026-11-07", "dinner"}} {
				portions = append(portions, map[string]any{"location": "fridge", "eat_on": slot[0], "eat_kind": slot[1]})
			}
			return fakellm.CallTool("check_plan", map[string]any{"prep_sessions": []map[string]any{{
				"on": "2026-11-06", "kind": "lunch", "ingredient_id": idIn(t, req, `Ingredient (\d+): yogurt`), "quantity": 150,
				"portions": portions,
			}}})
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Yogurt."),
	)
	s.OpensCycle(apptest.Cycle{Delivery: "11/06/2026", From: "10am", To: "12pm", Ends: "11/07/2026"})
	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")

	friday := "Friday, November 6 · lunch→ calories 360 protein 60"
	s.OpensUpcomingCycle()
	s.Sees(friday)
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Sees(friday)
}
