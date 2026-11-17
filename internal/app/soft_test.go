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
// misses; the Plan it submits is stored with its soft misses, which the Plan Proposal
// shows at its top, then the day table: the seven days before it, "No Plan" where no Plan
// covered them, each planned day against the Targets with each miss marked, and the
// average, calories against their range.
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
	s.Sees("- lasagne × 2 (freezer) left unused")
	s.Sees("- Rice bowl (freezer) left unused")

	s.Open("/plan")
	s.Sees("01 Plan Proposal Calories average 1586 a day against 2024–2376 " +
		"Protein averages 65 g a day against 130 g " +
		"Fri, Nov 6: 28% under calories Fri, Nov 6: 50% under protein " +
		"Rice bowl eaten back to back, Fri, Nov 6 " +
		"lasagne × 2 left unused in the freezer Rice bowl left unused in the freezer Friday.")
	s.Sees("Day Calories Protein Fibre Sugar Targets 2024–2376 at least 130 g at least 30 g at most 50 g")
	s.Sees("Fri, Oct 30 No Plan")
	s.Sees("Thu, Nov 5 No Plan Fri, Nov 6 1586 missed 65 g missed at least 4 g at least 8 g")
	s.Sees("Average 1586 missed 65 g missed at least 4 g at least 8 g")
}

// Changing the Targets once a Plan Proposal is made leaves what it shows unchanged: it
// keeps the Targets it aimed at.
func TestChangingTheTargetsLeavesAPlanProposalUnchanged(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Friday.")
	s.Open("/profile")
	s.SavesTargets(apptest.Targets{Kcal: "1500", Protein: "60", Fibre: "3", Sugar: "100", Tolerance: "10"})

	s.Open("/plan")
	s.Sees("Calories average 1586 a day against 2024–2376")
	s.Sees("Protein averages 65 g a day against 130 g")
	s.Sees("Targets 2024–2376 at least 130 g at least 30 g at most 50 g")
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
	s.Sees("Fri, Nov 6 1586 65 g at least 4 g at least 8 g Sat, Nov 7 360")
}

// A day a little under a Target is no soft miss, so the day table leaves it unmarked; the
// average missing it is marked, and listed. Friday's 65 g of protein is 7% under 70 g.
func TestADayALittleUnderATargetIsUnmarked(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Open("/profile")
	s.SavesTargets(apptest.Targets{Kcal: "1600", Protein: "70", Fibre: "30", Sugar: "50", Tolerance: "5"})
	plansFriday(t, s, "Friday.")

	s.Open("/plan")
	s.Sees("Protein averages 65 g a day against 70 g")
	s.DoesNotSee("under protein")
	s.Sees("Fri, Nov 6 1586 65 g at least 4 g at least 8 g Average 1586 65 g missed at least 4 g at least 8 g")
}
