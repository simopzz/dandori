package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// misoSalmon is a Recipe as draft_recipe takes it: the known rice, for which the model
// proposes values all the same, and salmon and miso, which are new.
func misoSalmon() map[string]any {
	return map[string]any{
		"name": "Miso salmon", "portions": 2, "active_minutes": 15, "total_minutes": 25,
		"slot_kinds": []string{"dinner"}, "appliances": []string{"oven"},
		"steps": []string{"Brush the salmon with miso.", "Bake it 12 minutes."},
		"ingredients": []map[string]any{
			{"name": "salmon", "quantity": 300, "unit": "g", "new": map[string]any{
				"kcal": 200, "protein_g": 20, "carbs_g": 0, "fat_g": 13, "fibre_g": 0, "sugar_g": 0,
				"usual": "fridge", "staple": false}},
			{"name": "miso", "quantity": 30, "unit": "g", "new": map[string]any{
				"kcal": 200, "protein_g": 12, "carbs_g": 26, "fat_g": 6, "fibre_g": nil, "sugar_g": nil,
				"usual": "fridge", "staple": false}},
			{"name": "rice", "quantity": 150, "unit": "g", "new": map[string]any{
				"kcal": 130, "protein_g": 3, "carbs_g": 28, "fat_g": 0, "fibre_g": 0, "sugar_g": 0,
				"usual": "pantry", "staple": false}},
		},
	}
}

// draftedDinner is fridayDraft with Friday's dinner a Portion of the Recipe Draft id,
// prepared then, the Rice bowl yielding only lunch and the frozen Portion.
func draftedDinner(t *testing.T, req agent.Request, id int64) map[string]any {
	t.Helper()
	d := fridayDraft(t, req)
	sessions, ok := d["prep_sessions"].([]map[string]any)
	if !ok { // not Fatalf: this runs in the Run's goroutine
		t.Errorf("fridayDraft has no Prep Sessions to add the dinner to: %v", d)
		return d
	}
	sessions[1]["portions"] = []map[string]any{
		{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "lunch"},
		{"location": "freezer"},
	}
	d["prep_sessions"] = append(sessions, map[string]any{
		"on": "2026-11-06", "kind": "dinner", "recipe_id": id, "size": 1, "portions": []map[string]any{
			{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "dinner"},
		},
	})
	return d
}

// plansMisoSalmon opens friday and plans it, through the Kitchen Check, into a pending
// Plan Proposal whose dinner is the planner's Recipe Draft of misoSalmon.
func plansMisoSalmon(t *testing.T, s *apptest.Scenario) {
	t.Helper()
	s.Model.Script(
		fakellm.CallTool("draft_recipe", misoSalmon()),
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", draftedDinner(t, req, -1)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Miso salmon for Friday's dinner."),
	)
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")
}

// The planner's Recipe Draft shows on its Plan Proposal: a line naming it and its new
// Ingredients, opening to its Ingredients, each new one marked with its proposed values,
// and its steps; the Slot it fills and its Prep Session say it is a new Recipe. It is
// never among the User's Recipe Drafts, nor in the Recipe Library.
func TestAPlannersRecipeDraftShowsOnThePlanProposalOnly(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansMisoSalmon(t, s)
	s.Sees("Drafted Recipe -1")

	s.Open("/plan")
	s.Sees("New Recipe: Miso salmon 2 new Ingredients")
	s.Sees("Miso salmon 1× · 593 kcal · 37 g protein Dinner new Recipe")
	s.Sees("Miso salmon new Recipe 1 at 1× 25 min · 15 active Ingredients salmon New 150 g miso New 15 g rice 75 g")
	sheet := s.HTMXGet(t, apptest.SheetSource(t, s.Page().Body, "New Recipe: Miso salmon"))
	if shown := apptest.Text(sheet.Body); sheet.StatusCode != http.StatusOK || strings.Contains(sheet.Body, "<html") || !strings.Contains(shown, "01 Miso salmon salmon New 300 g") {
		t.Errorf("the Recipe Draft's sheet answered %d:\n%s", sheet.StatusCode, sheet.Body)
	}

	s.Follows("New Recipe: Miso salmon")
	s.Sees("salmon New 300 g per 100 g: 200 kcal protein 20 g carbs 0 g fat 13 g fibre 0 g sugar 0 g")
	s.Sees("miso New 30 g per 100 g: 200 kcal protein 12 g carbs 26 g fat 6 g fibre – unknown sugar – unknown rice 150 g")
	s.Sees("Brush the salmon with miso.")

	s.Open("/recipes")
	s.DoesNotSee("Miso salmon")
	s.DoesNotSee("Recipe Draft")
}

// Approving the Plan Proposal adds its Recipe Draft to the Recipe Library as drafted,
// with its new Ingredients, and the Plan then cooks it as any Recipe.
func TestApprovingAPlanProposalAcceptsItsRecipeDraft(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansMisoSalmon(t, s)

	s.Open("/plan")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Sees("Miso salmon 1× · 593 kcal · 37 g protein Dinner cooked")
	s.DoesNotSee("new Recipe")

	s.Open("/recipes")
	s.Follows("Miso salmon")
	s.Sees("Makes 2 Portions Active 15 min Total 25 min Eaten at dinner Needs oven")
	s.Sees("salmon 300 g miso 30 g rice 150 g")
	s.Sees("Bake it 12 minutes.")
	s.Open("/ingredients")
	s.Sees("salmon Fridge per 100 g: 200 kcal protein 20 g")
	s.Sees("miso Fridge per 100 g: 200 kcal protein 12 g")
	if res := s.Get(t, "/plan/drafts/1"); res.StatusCode != http.StatusNotFound {
		t.Errorf("the accepted Recipe Draft opened with %d, want 404", res.StatusCode)
	}
}

// Rejecting the Plan Proposal discards its Recipe Draft: nothing reaches the Recipe
// Library or the Ingredients, and it no longer opens.
func TestRejectingAPlanProposalDiscardsItsRecipeDraft(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansMisoSalmon(t, s)

	s.Open("/plan")
	s.Submits("reject")
	s.IsConfirmed("Plan Proposal rejected.")
	s.Open("/recipes")
	s.DoesNotSee("Miso salmon")
	s.Open("/ingredients")
	s.DoesNotSee("salmon")
	s.DoesNotSee("miso")
	if res := s.Get(t, "/plan/drafts/1"); res.StatusCode != http.StatusNotFound {
		t.Errorf("the discarded Recipe Draft opened with %d, want 404", res.StatusCode)
	}
}

// draft_recipe tells the model what to fix, as submit_recipe does, and a name already in
// the Recipe Library; the model fixes them and drafts again. Drafting a name again
// replaces that draft, keeping its id.
func TestDraftRecipeProblemsGoBackToTheModel(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	taken := misoSalmon()
	taken["name"] = " rice BOWL "
	taken["ingredients"] = []map[string]any{{"name": "tofu", "quantity": 200, "unit": "g"}}
	smaller := misoSalmon()
	smaller["portions"] = 4
	s.Model.Script(
		fakellm.CallTool("draft_recipe", taken),
		fakellm.CallTool("draft_recipe", misoSalmon()),
		fakellm.CallTool("draft_recipe", smaller),
		fakellm.Then(func(req agent.Request) fakellm.Step { return fakellm.CallTool("check_plan", draftedDinner(t, req, -1)) }),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Miso salmon for Friday's dinner."),
	)
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Sees("fix these and submit again: ingredients.0.new: tofu is not a known Ingredient; propose its values in new " +
		"name: the Recipe Library already has Rice bowl, Recipe 1: prepare that one, or draft another name")
	s.Sees("Drafted Recipe -1: Miso salmon.")
	s.DoesNotSee("Drafted Recipe -2")

	s.Open("/plan")
	s.Sees("Miso salmon new Recipe 1 at 1× 25 min · 15 active Ingredients salmon New 75 g")
}

// Deleting a Recipe that a pending Plan Proposal, and then a Plan, prepares is refused,
// saying why, as a plain post and from its sheet; the Recipe stays in the Recipe Library.
func TestDeletingARecipeInAPlanIsRefused(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	refused := func() {
		t.Helper()
		s.Open("/recipes")
		s.Follows("Rice bowl")
		s.Follows("Edit")
		s.Follows("Delete")
		s.Submits("remove-step")
		s.Sees("Rice bowl is in a Plan or a Plan Proposal, so it can't be deleted.")
		s.Open("/recipes")
		s.Sees("Rice bowl")
	}
	refused()

	s.Open("/plan")
	s.Submits("approve")
	refused()
	s.Open("/recipes")
	s.Follows("Rice bowl")
	s.Open(s.Page().Path + "/edit")
	sheet := s.HTMXGet(t, apptest.SheetSource(t, s.Page().Body, "Delete"))
	step := apptest.Form(t, sheet.Body, "remove-step")
	got := s.HTMXPostForm(t, step.Action, step.Values)
	if got.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apptest.Text(got.Body), "so it can't be deleted") {
		t.Errorf("deleting from the sheet answered %d:\n%s", got.StatusCode, got.Body)
	}
}

// A Recipe Draft that no longer fits the Recipe Library, a Recipe of its name written
// since, refuses approving its Plan Proposal, saying why; nothing is accepted and it
// stays pending.
func TestApprovingIsRefusedWhenARecipeDraftClashes(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansMisoSalmon(t, s)
	clash := riceBowl()
	clash.Name = "Miso Salmon"
	s.WritesRecipe(clash)

	s.Open("/plan")
	s.Submits("approve")
	s.Sees("The new Recipe Miso salmon no longer fits your Recipe Library or Ingredients")
	s.Open("/plan")
	s.Sees("01 Plan Proposal")
	s.Open("/ingredients")
	s.DoesNotSee("salmon")
}

// Deleting a Recipe that only a rejected Plan Proposal prepares is refused too: the
// rejected Plan Proposal keeps its Prep Sessions, and they keep their Recipe.
func TestDeletingARecipeOnlyARejectedPlanProposalPreparesIsRefused(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFriday(t, s, "Rice bowls for Friday.")
	s.Open("/plan")
	s.Submits("reject")
	s.IsConfirmed("Plan Proposal rejected.")

	s.Open("/recipes")
	s.Follows("Rice bowl")
	s.Follows("Edit")
	s.Follows("Delete")
	s.Submits("remove-step")
	s.Sees("Rice bowl is in a Plan or a Plan Proposal, so it can't be deleted.")
}

// A Recipe Draft's new Ingredient the User has added since, with values of their own,
// refuses approving too: the Recipe Draft could no longer be accepted as drafted.
func TestApprovingIsRefusedWhenARecipeDraftsNewIngredientIsAddedSince(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansMisoSalmon(t, s)
	s.AddsIngredient(apptest.Ingredient{Name: "Miso", Unit: "g", Usual: "fridge", Kcal: "190", Protein: "11", Carbs: "25", Fat: "6"})

	s.Open("/plan")
	s.Submits("approve")
	s.Sees("The new Recipe Miso salmon no longer fits your Recipe Library or Ingredients")
	s.Open("/recipes")
	s.DoesNotSee("Miso salmon")
}

// Two Recipe Drafts proposing the same new Ingredient are both accepted: the first adds
// it, the second uses it.
func TestApprovingAcceptsRecipeDraftsSharingANewIngredient(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	soup := misoSalmon()
	soup["name"] = "Miso soup"
	s.Model.Script(
		fakellm.CallTool("draft_recipe", misoSalmon()),
		fakellm.CallTool("draft_recipe", soup),
		fakellm.Then(func(req agent.Request) fakellm.Step {
			d := draftedDinner(t, req, -1)
			sessions, _ := d["prep_sessions"].([]map[string]any)
			d["prep_sessions"] = append(sessions, map[string]any{
				"on": "2026-11-06", "kind": "dinner", "recipe_id": -2, "size": 1, "portions": []map[string]any{{"location": "freezer"}},
			})
			return fakellm.CallTool("check_plan", d)
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Miso salmon for Friday's dinner, and a miso soup for the freezer."),
	)
	s.OpensCycle(friday)
	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")

	s.Open("/plan")
	s.Submits("approve")
	s.IsConfirmed("Plan approved.")
	s.Open("/recipes")
	s.Sees("Miso salmon")
	s.Sees("Miso soup")
}

// A Recipe Draft's Portion eaten on a later day says where it is kept, as any Recipe's:
// only the Slot it is cooked for says it is a new Recipe.
func TestARecipeDraftsPortionEatenLaterSaysWhereItIsKept(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Model.Script(
		fakellm.CallTool("draft_recipe", misoSalmon()),
		fakellm.Then(func(req agent.Request) fakellm.Step {
			d := draftedDinner(t, req, -1)
			sessions, _ := d["prep_sessions"].([]map[string]any)
			sessions[1]["portions"] = []map[string]any{ // the Rice bowl's third Portion: Saturday's lunch, not the freezer
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "lunch"},
				{"location": "fridge", "eat_on": "2026-11-07", "eat_kind": "lunch"},
			}
			sessions[2]["portions"] = []map[string]any{
				{"location": "fridge", "eat_on": "2026-11-06", "eat_kind": "dinner"},
				{"location": "fridge", "eat_on": "2026-11-07", "eat_kind": "dinner"},
			}
			d["prep_sessions"] = append(sessions, map[string]any{
				"on": "2026-11-07", "kind": "breakfast", "ingredient_id": idIn(t, req, `Ingredient (\d+): yogurt`), "quantity": 150,
				"portions": []map[string]any{{"location": "fridge", "eat_on": "2026-11-07", "eat_kind": "breakfast"}},
			})
			kept, _ := d["kept"].([]map[string]any)
			d["kept"] = append(kept, map[string]any{"portion_id": idIn(t, req, `Portion (\d+): lasagne`) + 1, "on": "2026-11-07", "kind": "snack"})
			return fakellm.CallTool("check_plan", d)
		}),
		fakellm.CallTool("submit_plan", map[string]any{"draft_id": 1}),
		fakellm.Reply("Miso salmon for Friday's and Saturday's dinners."),
	)
	twoDays := friday
	twoDays.Ends = "11/07/2026"
	s.OpensCycle(twoDays)
	s.PlansCycle()
	s.Sees("Planning a Cycle Succeeded")

	s.Open("/plan")
	s.Sees("Miso salmon 1× · 593 kcal · 37 g protein Dinner new Recipe")
	s.Sees("Miso salmon 1× · 593 kcal · 37 g protein Dinner from Fri, Nov 6, in the fridge")
}
