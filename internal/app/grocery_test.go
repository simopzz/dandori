package app_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// sunday is a Cycle whose Delivery comes on Saturday 7 November between 6 and 8 pm,
// after friday's ends, its Slots all of Sunday 8.
var sunday = apptest.Cycle{Delivery: "11/07/2026", From: "6pm", To: "8pm", Ends: "11/08/2026"}

// stapleOil is olive oil marked a Staple, which no Grocery List buys.
var stapleOil = func() apptest.Ingredient { ing := oil; ing.Staple = true; return ing }()

// oiledRice is two Portions of 200 g rice and 10 ml olive oil as written.
func oiledRice() apptest.Recipe {
	r := riceBowl()
	r.Name = "Oiled rice"
	r.Ingredients = append(r.Ingredients, apptest.RecipeRow{Ingredient: apptest.Ingredient{Name: "olive oil"}, Quantity: "10"})
	return r
}

// sundayDraft is a draft Plan of sunday, read from the prompt of `req`: yogurt at
// breakfast and snack, 250 g in all, and Oiled rice prepared at lunch for three Portions
// at `size`, eaten at lunch and dinner with the third frozen.
func sundayDraft(t *testing.T, req agent.Request, size float64) map[string]any {
	t.Helper()
	rice := idIn(t, req, `Recipe (\d+): Oiled rice`)
	pot := idIn(t, req, `Ingredient (\d+): yogurt`)
	setOut := func(kind string, quantity int) map[string]any {
		return map[string]any{"on": "2026-11-08", "kind": kind, "ingredient_id": pot, "quantity": quantity, "portions": []map[string]any{
			{"location": "fridge", "eat_on": "2026-11-08", "eat_kind": kind},
		}}
	}
	return map[string]any{"prep_sessions": []map[string]any{
		setOut("breakfast", 150),
		{"on": "2026-11-08", "kind": "lunch", "recipe_id": rice, "size": size, "portions": []map[string]any{
			{"location": "fridge", "eat_on": "2026-11-08", "eat_kind": "lunch"},
			{"location": "fridge", "eat_on": "2026-11-08", "eat_kind": "dinner"},
			{"location": "freezer"},
		}},
		setOut("snack", 100),
	}}
}

// plansSunday scripts the model to plan sunday as sundayDraft does at `size`.
func plansSunday(t *testing.T, s *apptest.Scenario, size float64) {
	t.Helper()
	s.Model.Script(
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", sundayDraft(t, req, size)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Oiled rice for Sunday."),
	)
}

// sundayAfterFriday is ada's kitchen with friday's Plan approved, using up 375 g of the
// 800 g of rice on Friday, and sunday upcoming, planned at `size` and approved, landing
// on its Cycle page.
func sundayAfterFriday(t *testing.T, size float64) *apptest.Scenario {
	t.Helper()
	s := plannerWithKitchen(t, "ada@example.com")
	s.AddsIngredient(stapleOil)
	s.WritesRecipe(oiledRice())
	plansFriday(t, s, "Rice bowls for Friday.")
	s.Open("/plan")
	s.Submits("approve")
	s.At("Thu 20:00")
	plansSunday(t, s, size)
	s.OpensCycle(sunday)
	s.PlansCycle()
	s.Follows("Plan Proposal")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	return s
}

// An upcoming Cycle planned and approved links its Grocery List from the Cycle page: what
// its Plan needs beyond the kitchen forecast at its first Slot, once the current Cycle's
// Plan has used up its own, with the Staples it uses to check apart. Sunday's 450 g of
// rice less the 425 g Friday leaves is 25 g.
func TestAPlannedCyclesGroceryList(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	s.Follows("Grocery List")
	s.IsAt("/plan/cycles/2/grocery-list")
	s.Sees("Grocery List · Sat, Nov 7 – Sun, Nov 8")
	s.Sees("01 To buy rice (25 g) yogurt (250 g) 02 Check you still have olive oil")
	s.Follows("Back to the Cycle")
	s.IsAt("/plan/cycles/2")
}

// An approved replan changes the Grocery List: smaller Portions of Oiled rice need no
// more rice than Friday leaves. While the replan is pending, the Cycle page still links
// the list, which follows the Plan approved.
func TestAnApprovedReplanChangesTheGroceryList(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	plansSunday(t, s, 1.25)
	s.AsksForChanges("Smaller Portions.")
	s.Settle()
	s.Open("/plan/cycles/2")
	s.Sees("01 Plan Proposal")
	s.Follows("Grocery List")
	s.Sees("01 To buy rice (25 g) yogurt (250 g)")

	s.Open("/plan/cycles/2")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Follows("Grocery List")
	s.Sees("01 To buy yogurt (250 g) 02 Check you still have")
	s.DoesNotSee("rice (")
}

// The Grocery List of a Cycle without a Plan says to plan it first, linking back to the
// Cycle, which links no list.
func TestTheGroceryListOfACycleWithoutAPlan(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(friday)
	s.Open("/plan/cycles/1")
	s.DoesNotSee("Grocery List")
	s.Open("/plan/cycles/1/grocery-list")
	s.Sees("Plan the Cycle first: its Grocery List follows its Plan.")
	s.Follows("Back to the Cycle")
	s.IsAt("/plan/cycles/1")
}

// Only the upcoming and current Cycles of its own User have a Grocery List: another
// User's, and a closed Cycle's, lead to the Cycle page.
func TestOnlyTheUsersOpenCyclesHaveAGroceryList(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	s.At("Sat 20:00")
	s.Open("/plan/cycles/1/grocery-list")
	s.IsAt("/plan")
	s.Sees("Cycle · Sat, Nov 7 – Sun, Nov 8")

	s.Invite("bob@example.com")
	s.SwitchesTo("bob@example.com")
	s.ChoosesLanguage("English")
	s.Open("/plan/cycles/2/grocery-list")
	s.IsAt("/plan")
	s.Sees("No Cycle is upcoming.")
}

// Through a browser: the Cycle page links the Grocery List, whose lines tick off as the
// User fills the cart, a whole line being its checkbox's target.
func TestBrowserTicksOffTheGroceryList(t *testing.T) {
	s := sundayAfterFriday(t, 1.5)
	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/plan/cycles/2")
	b.Shot(t, "cycle-grocery-link")
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Grocery List"}).Click(); err != nil {
		t.Fatalf("follow the Grocery List: %v", err)
	}
	b.IsAt(t, "/plan/cycles/2/grocery-list")
	b.SeesInTurn(t, "Grocery List", "01 To buy", "rice (25 g)", "yogurt (250 g)", "02 Check you still have", "olive oil")

	expect := playwright.NewPlaywrightAssertions()
	rice := b.GetByRole("checkbox", playwright.PageGetByRoleOptions{Name: "rice (25 g)"})
	if err := rice.Check(); err != nil {
		t.Fatalf("tick rice: %v", err)
	}
	if err := b.GetByText("yogurt").Click(); err != nil {
		t.Fatalf("tick yogurt by its name: %v", err)
	}
	if err := expect.Locator(rice).ToBeChecked(); err != nil {
		t.Errorf("rice isn't ticked: %v", err)
	}
	yogurt := b.GetByRole("checkbox", playwright.PageGetByRoleOptions{Name: "yogurt (250 g)"})
	if err := expect.Locator(yogurt).ToBeChecked(); err != nil {
		t.Errorf("yogurt isn't ticked by its line: %v", err)
	}
	b.Shot(t, "grocery-list")
}
