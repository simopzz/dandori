package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// monday is a Cycle whose Delivery comes on Monday 2 November between 6 and 7 am, the
// day the clock starts at midnight, and that ends on Tuesday 3: its Slots are Monday's
// four and Tuesday's.
var monday = apptest.Cycle{Delivery: "11/02/2026", From: "6am", To: "7am", Ends: "11/03/2026"}

// mondayDraft is a draft Plan of monday, read from the prompt of req: yogurt prepared at
// Monday's breakfast fills every Slot but the frozen lasagne's, at lasagneOn's
// lasagneKind; with bowlAtBreakfast, a Rice bowl prepared then fills breakfast instead.
func mondayDraft(t *testing.T, req agent.Request, lasagneOn, lasagneKind string, bowlAtBreakfast bool) map[string]any {
	t.Helper()
	pot := idIn(t, req, `Ingredient (\d+): yogurt`)
	lasagne := idIn(t, req, `Portion (\d+): lasagne`)
	var pots []map[string]any
	for _, on := range []string{"2026-11-02", "2026-11-03"} {
		for _, kind := range []string{"breakfast", "lunch", "snack", "dinner"} {
			if (on == lasagneOn && kind == lasagneKind) || (bowlAtBreakfast && on == "2026-11-02" && kind == "breakfast") {
				continue
			}
			pots = append(pots, map[string]any{"location": "fridge", "eat_on": on, "eat_kind": kind})
		}
	}
	sessions := []map[string]any{
		{"on": "2026-11-02", "kind": "breakfast", "ingredient_id": pot, "quantity": 150, "portions": pots},
	}
	if bowlAtBreakfast {
		sessions = append(sessions, map[string]any{
			"on": "2026-11-02", "kind": "breakfast", "recipe_id": idIn(t, req, `Recipe (\d+): Rice bowl`), "size": 1,
			"portions": []map[string]any{{"location": "fridge", "eat_on": "2026-11-02", "eat_kind": "breakfast"}},
		})
	}
	return map[string]any{
		"prep_sessions": sessions,
		"kept":          []map[string]any{{"portion_id": lasagne, "on": lasagneOn, "kind": lasagneKind}},
	}
}

// check_plan reports a Recipe at a kind of Slot it isn't for, a Recipe needing an
// appliance the User doesn't own, and frozen food with no evening left to thaw it, and
// submit_plan refuses a draft breaking them.
func TestCheckPlanReportsKindsOfSlotAppliancesAndThawAndSubmitPlanRefusesThem(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Open("/profile")
	s.SavesAppliances([]string{"hob", "oven"}, nil)
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			return fakellm.CallTool("check_plan", mondayDraft(t, req, "2026-11-02", "dinner", true))
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("I give up."),
	)
	s.OpensCycle(monday)
	s.PlansCycle()

	s.Sees("Draft 1 breaks these hard rules:")
	s.Sees("wrong-kind-of-slot: Rice bowl is eaten at breakfast on Mon 2 Nov, a kind of Slot it isn't for: lunch, dinner")
	s.Sees("appliance-not-owned: Rice bowl at breakfast on Mon 2 Nov needs the appliance microwave, which the user doesn't own")
	s.Sees("no-evening-to-thaw: Portion 1 is frozen, and the evening before dinner on Mon 2 Nov to thaw it has passed")
	s.Sees("draft 1 breaks these hard rules (submit 1 of 3): - wrong-kind-of-slot")
	s.Sees("The model ended without a Plan Proposal. I give up.")
}

// A frozen Portion in the kitchen put in a Slot whose evening before has passed is
// refused; put in one the next day, the Plan keeps every hard rule.
func TestAFrozenPortionNeedsAnEveningLeftToThaw(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			return fakellm.CallTool("check_plan", mondayDraft(t, req, "2026-11-02", "dinner", false))
		}),
		fakellm.Then(func(req agent.Request) fakellm.Step {
			return fakellm.CallTool("check_plan", mondayDraft(t, req, "2026-11-03", "lunch", false))
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 2}),
		fakellm.Reply("Yogurt, and the lasagne on Tuesday."),
	)
	s.OpensCycle(monday)
	s.PlansCycle()

	s.Sees("Draft 1 breaks these hard rules: - no-evening-to-thaw: Portion 1 is frozen, and the evening before dinner on Mon 2 Nov to thaw it has passed")
	s.Sees("Draft 2 keeps every hard rule.")
	s.Sees("Planning a Cycle Succeeded")
}

// The freezer splits a Plan names are shown under its Delivery's day, flagged while it
// is folded, on the Plan Proposal and on the Plan once approved.
func TestFreezerSplitsShowUnderTheDeliverysDay(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.AddsIngredient(chicken)
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			d := fridayDraft(t, req)
			d["freezer_splits"] = []map[string]any{{"ingredient_id": idIn(t, req, `Ingredient (\d+): chicken breast`), "quantity": 400}}
			return fakellm.CallTool("check_plan", d)
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Rice bowls, and chicken frozen for later."),
	)
	s.OpensCycle(friday)
	s.PlansCycle()

	s.Open("/plan")
	s.Sees("Thursday, November 5 1 to the freezer at put-away To the freezer at put-away chicken breast 400 g Friday, November 6")

	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Sees("Thursday, November 5 1 to the freezer at put-away To the freezer at put-away chicken breast 400 g Friday, November 6")
}
