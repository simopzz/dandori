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

// check_plan gives the planner each day's nutrition, the Targets' results and the soft
// misses, then the Portions made for after the Cycle as information; the Plan it submits
// is stored with its soft misses. The Plan Proposal shows first the average under heads
// naming each Target's unit and bounds, each miss marked under or over and a floor
// written "4+", then the soft misses grouped by kind with that information; then its
// days under the same heads: the seven days before it, those no Plan covered as one
// range, and each planned day with its misses marked, what it cooks, its Slots and the
// Prep Sessions it holds.
func TestAPlanProposalShowsItsSoftMissesAndItsDays(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")
	s.Sees("Draft 1 keeps every hard rule.")
	s.Sees("Fri 6 Nov: 1586 kcal (missed), 65 g protein (missed), at least 4 g fibre, at least 8 g sugar")
	s.Sees("Average over the 1 day judged: 1586 kcal (missed), 65 g protein (missed), at least 4 g fibre, at least 8 g sugar")
	s.Sees("- Calories average 1586 a day against 2024–2376")
	s.Sees("- Protein averages 65 g a day against 130 g")
	s.Sees("- Fri 6 Nov: 28% under kcal")
	s.Sees("- Fri 6 Nov: 50% under protein")
	s.Sees("- Rice bowl eaten back to back, Fri 6 Nov")
	s.Sees("- lasagne: 2 of 3 left unused, in the freezer")
	s.Sees("Information, nothing to fix: 1 Portion made for after the Cycle: Rice bowl")

	s.Open("/plan")
	s.Sees("01 Plan Proposal Friday. Average " + heads +
		" calories 1586 under protein 65 under fibre at least 4 + sugar at least 8 + A day is marked")
	s.Sees("1 day more than 25% off Fri, Nov 6: calories under protein under " +
		"Rice bowl eaten back to back, Fri, Nov 6 " +
		"lasagne: 2 of 3 left unused, in the freezer 1 Portion made for after the Cycle: Rice bowl Approve")
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
	if p := reqs[len(reqs)-3].Messages[0].Content[0].Text; !strings.Contains(p, "- Fri 6 Nov: 1586 kcal, 65 g protein, at least 4 g fibre, at least 8 g sugar") ||
		!strings.Contains(p, "- Thu 5 Nov: no Plan") {
		t.Errorf("the planner isn't told the seven days before:\n%s", p)
	}

	s.Open("/plan")
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
