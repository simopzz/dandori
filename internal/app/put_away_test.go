package app_test

import (
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// plansFridaySplitting opens friday, plans it as fridayDraft with 100 g of yogurt and 400
// g of chicken breast, which it adds and no Prep Session uses, split to the freezer, and
// approves it. Its Grocery List buys 150 g of yogurt.
func plansFridaySplitting(t *testing.T, s *apptest.Scenario) {
	t.Helper()
	s.AddsIngredient(chicken)
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step {
			d := fridayDraft(t, req)
			d["freezer_splits"] = []map[string]any{
				{"ingredient_id": idIn(t, req, `Ingredient (\d+): yogurt`), "quantity": 100},
				{"ingredient_id": idIn(t, req, `Ingredient (\d+): chicken breast`), "quantity": 400},
			}
			return fakellm.CallTool("check_plan", d)
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Rice bowls, and some yogurt frozen."),
	)
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Open("/plan")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
}

// At the end of the Delivery's window its Grocery List becomes Stock Items: yogurt in the
// fridge but for the 100 g its freezer split sends to the freezer, and no chicken, which
// the list doesn't buy. Their Movements say delivered. The current Cycle's list stays as
// it arrived, saying "Delivered on" its day, with neither the hint to fill the cart nor
// checkboxes, offering neither marking ordered nor undoing it, which are refused,
// saying why.
func TestTheDeliveryIsPutAwayWhenItArrives(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.Follows("Grocery List")
	s.Sees("01 To buy yogurt (150 g)")
	mark := apptest.Form(t, s.Page().Body, "mark-ordered")
	s.Submits("mark-ordered")
	undo := apptest.Form(t, s.Page().Body, "undo-ordered")

	s.At("Thu 20:00")
	s.Open("/kitchen")
	s.Sees("Fridge yogurt 50 g Count yogurt in the fridge Move yogurt in the fridge")
	s.Sees("Move lasagne in the freezer yogurt 100 g Count yogurt in the freezer Move yogurt in the freezer")
	s.DoesNotSee("Count chicken breast")
	s.Open("/kitchen/movements?location=freezer&ingredient=" + ingredientID(t, s, "yogurt"))
	s.Sees("Delivered 0 g → 100 g Thu, Nov 5 · 8:00 pm")

	s.Open("/plan/cycles/1")
	s.Sees("Delivered on Thu, Nov 5")
	s.DoesNotSee("Never marked ordered")
	s.Follows("Grocery List")
	s.Sees("Delivered on Thu, Nov 5")
	s.Sees("01 To buy yogurt 150 g")
	s.DoesNotSee("Tick each line as you fill the cart.")
	if strings.Contains(s.Page().Body, `type="checkbox"`) {
		t.Error("the Grocery List of a Delivery put away has a checkbox")
	}
	s.DoesNotSee("Mark ordered")
	s.DoesNotSee("Undo ordered")
	const arrived = "The Delivery has arrived: its Grocery List stays as it came."
	refusedWith(t, s.PostForm(t, mark.Action, mark.Values), arrived)
	refusedWith(t, s.PostForm(t, undo.Action, undo.Values), arrived)

	// A replan approved after arrival leaves the list as it arrived.
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", fridayDraft(t, req)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Rice bowls again."),
	)
	s.Open("/plan/cycles/1")
	s.AsksForChanges("Again.")
	s.Settle()
	s.Open("/plan/cycles/1")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Follows("Grocery List")
	s.Sees("01 To buy yogurt 150 g")
}

// A Delivery is put away once, however many beats and requests pass its window's end.
func TestADeliveryIsPutAwayOnce(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.At("Thu 19:58")
	s.Open("/kitchen")
	s.DoesNotSee("Count yogurt")
	s.At("Thu 20:00")
	s.Open("/kitchen")
	s.At("Thu 20:01")
	s.Open("/kitchen")
	s.At("Thu 21:30")
	s.Open("/kitchen")
	s.Open("/kitchen/movements?location=fridge&ingredient=" + ingredientID(t, s, "yogurt"))
	s.Sees("yogurt · Fridge Delivered 0 g → 50 g Thu, Nov 5 · 8:00 pm")
	s.DoesNotSee("Delivered 50 g")
}

// A Delivery never marked ordered delivers its Grocery List as the Plan had it at
// arrival, and its Cycle page and list say it was never marked ordered.
func TestADeliveryNeverMarkedOrderedIsPutAwayAllTheSame(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.At("Thu 20:05")
	s.Open("/kitchen")
	s.Sees("Fridge yogurt 50 g")
	s.Open("/plan/cycles/1")
	s.Sees("Delivered on Thu, Nov 5 Never marked ordered")
	s.Follows("Grocery List")
	s.Sees("Delivered on Thu, Nov 5 Never marked ordered")
	s.Sees("01 To buy yogurt 150 g")
}

// A Cycle whose Delivery arrives without a Plan delivers nothing.
func TestACycleArrivingWithoutAPlanDeliversNothing(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.OpensCycle(friday)
	s.At("Thu 20:05")
	s.Open("/kitchen")
	s.Sees("rice 800 g")
	s.DoesNotSee("Count yogurt")
	s.Open("/plan/cycles/1")
	s.Sees("Delivered on Thu, Nov 5 Never marked ordered")
}
