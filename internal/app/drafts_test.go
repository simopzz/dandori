package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// draftedLasagne is a User who has rice and a parse Run's Recipe Draft of
// submittedLasagne, open on its Run's page.
func draftedLasagne(t *testing.T) *apptest.Scenario {
	t.Helper()
	s := kitchenWith(t, rice)
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)
	return s
}

// A Recipe Draft opens in the Recipe form holding what was parsed, each new Ingredient's
// proposed values shown to confirm, and accepting it as shown adds the Recipe with its
// new Ingredients and removes the Recipe Draft, which its Run no longer links to.
func TestAcceptingARecipeDraftAddsItAndItsNewIngredients(t *testing.T) {
	s := draftedLasagne(t)
	run := s.Page().Path

	s.Follows("Recipe Draft")
	s.Sees("Recipe Draft Name Portions")
	draftHolds(t, s, map[string]string{"name": "Lasagne", "ing_name_0": "beef mince", "ing_name_4": "rice"})
	draft := s.Page().Path

	s.Submits("recipe-form")
	s.IsConfirmed("Added Lasagne to the Recipe Library.")
	s.Sees("Makes 4 Portions Active 30 min Total 70 min Eaten at lunch, dinner Needs oven, hob")
	s.Sees("beef mince 500 g")
	s.Sees("rice 100 g")
	s.Open("/ingredients")
	s.Sees("lasagne sheets")
	s.Sees("olive oil")

	if res := s.Get(t, draft); res.StatusCode != http.StatusNotFound {
		t.Errorf("the accepted Recipe Draft opened with %d, want 404", res.StatusCode)
	}
	s.Open(run)
	if strings.Contains(s.Page().Body, draft) {
		t.Errorf("the Run still links to its accepted Recipe Draft")
	}
}

// A Recipe Draft is corrected in the form before it is accepted: its name, a row removed,
// a row added for a new Ingredient, and a step changed. Only the corrected Recipe lands.
func TestARecipeDraftIsCorrectedBeforeItIsAccepted(t *testing.T) {
	s := draftedLasagne(t)
	s.Follows("Recipe Draft")

	form := apptest.Form(t, s.Page().Body, "recipe-form")
	form.Values.Set("name", "Beef lasagne")
	form.Values.Set("ing_name_2", "") // chopped tomatoes, cleared to remove it
	form.Values.Set("ing_quantity_2", "")
	for field, v := range map[string]string{
		"name": "tofu", "quantity": "300", "new": "true", "unit": "g", "usual": "fridge",
		"kcal": "120", "protein": "12", "carbs": "2", "fat": "7",
	} {
		form.Values.Set("ing_"+field+"_5", v)
	}
	form.Values["steps"] = []string{"Brown the mince.", "Layer it with the sheets.", "Bake 40 minutes."}
	res := s.PostForm(t, form.Action, form.Values)
	s.Open(res.Path)

	s.Sees("Beef lasagne")
	s.Sees("tofu 300 g")
	s.DoesNotSee("chopped tomatoes")
	s.Sees("Brown the mince. Layer it with the sheets. Bake 40 minutes.")
	s.Open("/ingredients")
	s.Sees("tofu")
	s.DoesNotSee("chopped tomatoes")
}

// Accepting is refused as writing a Recipe is: a name the User has already, with a link
// to it, or a new Ingredient's value cleared. A refused accept changes nothing, and the
// Recipe Draft waits.
func TestARefusedAcceptChangesNothing(t *testing.T) {
	s := draftedLasagne(t)
	draft := apptest.Link(t, s.Page().Body, "Recipe Draft")
	r := riceBowl()
	r.Name = "lasagne "
	s.WritesRecipe(r)

	s.Open(draft)
	s.Submits("recipe-form")
	s.IsRefused()
	s.Sees("You already have a Recipe with this name: lasagne")

	s.Open(draft)
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	form.Values.Set("name", "Lasagne al forno")
	form.Values.Set("ing_kcal_0", "")
	if res := s.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("an accept with a value cleared answered %d, want 422", res.StatusCode)
	}

	s.Open("/ingredients")
	s.DoesNotSee("beef mince")
	s.Open("/recipes")
	s.DoesNotSee("Lasagne al forno")
	s.Open(draft)
	s.Sees("Recipe Draft Name Portions")
}

// A known Ingredient keeps its own values and unit on the Recipe Draft, whatever the
// model proposed for it, and accepting leaves them as they were.
func TestAKnownIngredientKeepsItsValuesAndUnitOnARecipeDraft(t *testing.T) {
	s := kitchenWith(t, rice)
	sub := submittedLasagne()
	ings := sub["ingredients"].([]map[string]any)
	ings[4]["new"] = map[string]any{
		"kcal": 130, "protein_g": 3, "carbs_g": 28, "fat_g": 0.3, "fibre_g": nil, "sugar_g": nil, "usual": "fridge", "staple": false,
	}
	s.Model.Script(fakellm.CallTool("submit_recipe", sub), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)
	s.Follows("Recipe Draft")

	s.Sees("Measured in g")
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	if got := form.Get("ing_kcal_4"); got != "" {
		t.Errorf("the known rice shows the proposed %q kcal", got)
	}
	s.Submits("recipe-form")
	s.IsConfirmed("Added Lasagne to the Recipe Library.")
	s.Open("/ingredients")
	s.Sees("rice pantry per 100 g: 350 kcal protein 7 g")
}

// Discarding a Recipe Draft removes it and adds nothing, and its Run stops linking
// to it.
func TestDiscardingARecipeDraftRemovesIt(t *testing.T) {
	s := draftedLasagne(t)
	run := s.Page().Path
	s.Follows("Recipe Draft")
	draft := s.Page().Path

	s.Submits("recipe-remove")
	s.IsAt("/recipes")
	s.IsConfirmed("Discarded the Recipe Draft Lasagne.")
	s.Sees("No Recipes yet.")
	s.DoesNotSee("Recipe Drafts to check")
	if res := s.Get(t, draft); res.StatusCode != http.StatusNotFound {
		t.Errorf("the discarded Recipe Draft opened with %d, want 404", res.StatusCode)
	}
	s.Open("/ingredients")
	s.DoesNotSee("beef mince")
	s.Open(run)
	if strings.Contains(s.Page().Body, draft) {
		t.Errorf("the Run still links to its discarded Recipe Draft")
	}
}

// Every Recipe Draft awaiting a check is listed on the Recipe Library, newest first and
// before the Recipes, with the day it was read, until it is accepted or discarded.
func TestRecipeDraftsToCheckAreListedFirstOnTheRecipeLibrary(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)
	s.At("Wed 18:00")
	curry := submittedLasagne()
	curry["name"] = "Curry"
	s.Model.Script(fakellm.CallTool("submit_recipe", curry), fakellm.Reply("Done."))
	s.PastesRecipe("Curry for 4")

	s.Open("/recipes")
	s.Sees("Recipe Drafts to check Curry read Wed 4 Nov Lasagne read Mon 2 Nov Recipes Search Rice bowl")
	s.Follows("Lasagne")
	s.Sees("Recipe Draft Name Portions")
	s.Submits("recipe-form")
	s.Open("/recipes")
	s.Follows("Curry")
	s.Submits("recipe-remove")

	s.Open("/recipes")
	s.DoesNotSee("Recipe Drafts to check")
	s.Sees("Lasagne")
}

// A Recipe Draft fixes the User's Language, as a Recipe does.
func TestARecipeDraftFixesTheLanguage(t *testing.T) {
	s := kitchenWith(t)
	sub := submittedLasagne()
	sub["ingredients"] = sub["ingredients"].([]map[string]any)[:4] // all new, so the User has no Ingredient
	s.Model.Script(fakellm.CallTool("submit_recipe", sub), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)

	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.IsRefused()
	s.Sees("Your kitchen already holds Ingredients, Recipes, Recipe Drafts or Portions in this Language, so it stays.")
}

// draftHolds checks the Recipe form open on s holds each field's value.
func draftHolds(t *testing.T, s *apptest.Scenario, want map[string]string) {
	t.Helper()
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	for field, v := range want {
		if got := form.Get(field); got != v {
			t.Errorf("the form's %s is %q, want %q", field, got, v)
		}
	}
}
