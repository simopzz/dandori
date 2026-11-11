package app_test

import (
	"net/http"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// lasagnaSheets is a known Ingredient a slip away from the parsed "lasagne sheets".
var lasagnaSheets = apptest.Ingredient{Name: "lasagna sheets", Unit: "g", Usual: "pantry", Kcal: "360", Protein: "13", Carbs: "72", Fat: "1.5"}

// draftedLasagneWith is a User who has ings and a Recipe Draft of submittedLasagne, open
// on its form.
func draftedLasagneWith(t *testing.T, ings ...apptest.Ingredient) *apptest.Scenario {
	t.Helper()
	s := kitchenWith(t, ings...)
	s.Model.Script(fakellm.CallTool("submit_recipe", submittedLasagne()), fakellm.Reply("Done."))
	s.PastesRecipe(pastedLasagne)
	s.Follows("Recipe Draft")
	return s
}

// A parsed name close to a known Ingredient's suggests it. Ignoring the suggestion keeps
// the new Ingredient, which accepting adds beside the known one.
func TestIgnoringASuggestionKeepsTheNewIngredient(t *testing.T) {
	s := draftedLasagneWith(t, rice, lasagnaSheets)
	draftHolds(t, s, map[string]string{"ing_name_1": "lasagne sheets"})
	s.Sees("Did you mean lasagna sheets? Use lasagna sheets")

	s.Submits("recipe-form")
	s.IsConfirmed("Added Lasagne to the Recipe Library.")
	s.Sees("lasagne sheets 250 g")
	s.Open("/ingredients")
	s.Sees("lasagna sheets")
	s.Sees("lasagne sheets")
}

// Accepting a suggestion in the known Ingredient's own unit uses it, quantity kept, and
// leaves its values as they were. It works in place with htmx and as a plain post.
func TestAcceptingASuggestionInTheSameUnitUsesTheKnownIngredient(t *testing.T) {
	s := draftedLasagneWith(t, rice, lasagnaSheets)
	form := apptest.Form(t, s.Page().Body, "recipe-form")

	row := s.HTMXGet(t, "/recipes/ingredient-row?ing_pick_1=lasagna+sheets&ing_name_1=lasagne+sheets&ing_quantity_1=250&ing_unit_1=g&ing_new_1=true")
	if got := apptest.Text(row.Body); got != "Ingredient Quantity Measured in g" {
		t.Errorf("the picked row shows %q", got)
	}
	if got := apptest.Form(t, "<form id=r>"+row.Body+"</form>", "r"); got.Get("ing_name_1") != "lasagna sheets" || got.Get("ing_quantity_1") != "250" {
		t.Errorf("the picked row holds %q, %q", got.Get("ing_name_1"), got.Get("ing_quantity_1"))
	}

	// the row gets the Ingredient the button named, not whichever is closest when clicked
	rice := s.HTMXGet(t, "/recipes/ingredient-row?ing_pick_1=rice&ing_name_1=lasagne+sheets&ing_quantity_1=250&ing_unit_1=g&ing_new_1=true")
	if got := apptest.Form(t, "<form id=r>"+rice.Body+"</form>", "r").Get("ing_name_1"); got != "rice" {
		t.Errorf("picking rice gave the row %q", got)
	}
	gone := s.HTMXGet(t, "/recipes/ingredient-row?ing_pick_1=tofu&ing_name_1=lasagne+sheets&ing_quantity_1=250&ing_unit_1=g&ing_new_1=true")
	if got := apptest.Form(t, "<form id=r>"+gone.Body+"</form>", "r").Get("ing_name_1"); got != "lasagne sheets" {
		t.Errorf("picking an Ingredient the User hasn't got gave the row %q", got)
	}

	form.Values.Set("ing_pick_1", "lasagna sheets")
	picked := s.PostForm(t, form.Action, form.Values)
	if picked.StatusCode != http.StatusOK {
		t.Fatalf("picking answered %d, want 200", picked.StatusCode)
	}
	draft := apptest.Form(t, picked.Body, "recipe-form")
	if draft.Get("ing_name_1") != "lasagna sheets" || draft.Get("ing_quantity_1") != "250" {
		t.Errorf("the picked row holds %q, %q", draft.Get("ing_name_1"), draft.Get("ing_quantity_1"))
	}
	res := s.PostForm(t, draft.Action, draft.Values)
	s.Open(res.Path)
	s.Sees("lasagna sheets 250 g")
	s.Open("/ingredients")
	s.DoesNotSee("lasagne sheets")
	s.Sees("lasagna sheets pantry per 100 g: 360 kcal")
}

// Accepting a suggestion measured in another unit switches the row to it and clears the
// quantity: Dandori never converts, so the Recipe Draft waits for one in that unit.
func TestAcceptingASuggestionInAnotherUnitAsksForTheQuantity(t *testing.T) {
	sheets := lasagnaSheets
	sheets.Unit = "piece"
	s := draftedLasagneWith(t, rice, sheets)
	form := apptest.Form(t, s.Page().Body, "recipe-form")

	row := s.HTMXGet(t, "/recipes/ingredient-row?ing_pick_1=lasagna+sheets&ing_name_1=lasagne+sheets&ing_quantity_1=250&ing_unit_1=g&ing_new_1=true")
	if got := apptest.Text(row.Body); got != "Ingredient Quantity Give it in pieces: Dandori doesn't convert. Measured in piece" {
		t.Errorf("the picked row shows %q", got)
	}

	form.Values.Set("ing_pick_1", "lasagna sheets")
	picked := s.PostForm(t, form.Action, form.Values)
	draft := apptest.Form(t, picked.Body, "recipe-form")
	if draft.Get("ing_quantity_1") != "" {
		t.Errorf("the picked row kept the quantity %q", draft.Get("ing_quantity_1"))
	}
	if res := s.PostForm(t, draft.Action, draft.Values); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("accepting with no quantity answered %d, want 422", res.StatusCode)
	}
	draft.Values.Set("ing_quantity_1", "12")
	res := s.PostForm(t, draft.Action, draft.Values)
	s.Open(res.Path)
	s.Sees("lasagna sheets 12 pieces")
}

// A row whose new Ingredient the User has since added under that name uses the known one,
// whether the form was opened before or after: in the same unit as it is, in another
// with its quantity asked for again.
func TestARecipeDraftUsesAnIngredientAddedSince(t *testing.T) {
	s := draftedLasagneWith(t, rice)
	draft := s.Page().Path
	before := apptest.Form(t, s.Page().Body, "recipe-form")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "Beef mince", Unit: "g", Usual: "freezer", Kcal: "200", Protein: "20", Carbs: "0", Fat: "12"})
	s.AddsIngredient(apptest.Ingredient{Name: "chopped tomatoes", Unit: "piece", Usual: "pantry", Kcal: "80", Protein: "4", Carbs: "14", Fat: "1"})

	s.Open(draft)
	s.Sees("Give it in pieces: Dandori doesn't convert.")
	after := apptest.Form(t, s.Page().Body, "recipe-form")
	if after.Get("ing_name_0") != "Beef mince" || after.Get("ing_kcal_0") != "" || after.Get("ing_quantity_2") != "" {
		t.Errorf("the reopened draft holds %q, kcal %q, tomatoes %q", after.Get("ing_name_0"), after.Get("ing_kcal_0"), after.Get("ing_quantity_2"))
	}

	refused := s.PostForm(t, before.Action, before.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("accepting the form opened before answered %d, want 422", refused.StatusCode)
	}
	form := apptest.Form(t, refused.Body, "recipe-form")
	if form.Get("ing_quantity_2") != "" {
		t.Errorf("the tomatoes kept the quantity %q", form.Get("ing_quantity_2"))
	}
	form.Values.Set("ing_quantity_2", "2")
	res := s.PostForm(t, form.Action, form.Values)
	s.Open(res.Path)
	s.Sees("Beef mince 500 g")
	s.Sees("chopped tomatoes 2 pieces")
	s.Open("/ingredients")
	s.Sees("Beef mince freezer per 100 g: 200 kcal")
}

// A row whose known Ingredient the User has since deleted asks for its values like a
// new one, whether the form was opened before or after.
func TestARecipeDraftAsksForAnIngredientDeletedSince(t *testing.T) {
	s := draftedLasagneWith(t, rice)
	draft := s.Page().Path
	before := apptest.Form(t, s.Page().Body, "recipe-form")
	s.Open("/ingredients")
	s.DeletesIngredient("rice")

	if res := s.PostForm(t, before.Action, before.Values); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("accepting the form opened before answered %d, want 422", res.StatusCode)
	}
	s.Open(draft)
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	if form.Get("ing_name_4") != "rice" || form.Get("ing_unit_4") != "g" || form.Get("ing_quantity_4") != "100" {
		t.Errorf("the rice row holds %q, %q, %q", form.Get("ing_name_4"), form.Get("ing_unit_4"), form.Get("ing_quantity_4"))
	}
	if res := s.PostForm(t, form.Action, form.Values); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("accepting rice with no values answered %d, want 422", res.StatusCode)
	}
	for field, v := range map[string]string{"usual": "pantry", "kcal": "350", "protein": "7", "carbs": "78", "fat": "0.6"} {
		form.Values.Set("ing_"+field+"_4", v)
	}
	res := s.PostForm(t, form.Action, form.Values)
	s.Open(res.Path)
	s.Sees("rice 100 g")
}

// In a browser, picking a suggestion swaps its row in place to the known Ingredient,
// its quantity asked for again in the known unit.
func TestBrowserPicksASuggestionInPlace(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	browserHasLasagnaSheets(t, b)

	b.Open(t, "/recipes/new")
	name := b.Locator(`[name="ing_name_0"]`)
	if err := name.Fill("lasagne sheets"); err != nil {
		t.Fatalf("name the row: %v", err)
	}
	if err := b.Locator(`[name="ing_quantity_0"]`).Fill("250"); err != nil {
		t.Fatalf("give a quantity: %v", err)
	}
	if _, err := b.Locator(`[name="ing_unit_0"]`).SelectOption(playwright.SelectOptionValues{Values: &[]string{"g"}}); err != nil {
		t.Fatalf("choose g: %v", err)
	}
	b.Click(t, "Use lasagna sheets")
	b.Sees(t, "Give it in pieces: Dandori doesn't convert.")
	if err := playwright.NewPlaywrightAssertions().Locator(name).ToHaveValue("lasagna sheets"); err != nil {
		t.Error(err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(b.Locator(`[name="ing_quantity_0"]`)).ToHaveValue(""); err != nil {
		t.Error(err)
	}
	b.Shot(t, "suggestion-picked")
}

// browserHasLasagnaSheets adds lasagna sheets, measured in pieces, in the browser.
func browserHasLasagnaSheets(t *testing.T, b *apptest.Browser) {
	t.Helper()
	b.Open(t, "/ingredients")
	form := b.Locator("#ingredient-add")
	for field, v := range map[string]string{"name": "lasagna sheets", "kcal": "360", "protein": "13", "carbs": "72", "fat": "1.5"} {
		if err := form.Locator(`[name="` + field + `"]`).Fill(v); err != nil {
			t.Fatalf("fill %s: %v", field, err)
		}
	}
	for field, v := range map[string]string{"unit": "piece", "usual": "pantry"} {
		if _, err := form.Locator(`[name="` + field + `"]`).SelectOption(playwright.SelectOptionValues{Values: &[]string{v}}); err != nil {
			t.Fatalf("choose %s: %v", field, err)
		}
	}
	if err := form.Locator(`[type="submit"]`).Click(); err != nil {
		t.Fatalf("add: %v", err)
	}
	b.Sees(t, "lasagna sheets")
}

// In a browser, Enter in the Recipe form saves it, even with a suggestion and the add
// buttons above the save button.
func TestBrowserEnterSavesTheRecipe(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	browserHasLasagnaSheets(t, b)

	b.Open(t, "/recipes/new")
	for field, v := range map[string]string{"portions": "2", "active": "10", "total": "20", "ing_name_0": "lasagna sheets", "ing_quantity_0": "4"} {
		if err := b.Locator(`[name="` + field + `"]`).Fill(v); err != nil {
			t.Fatalf("fill %s: %v", field, err)
		}
	}
	if err := b.Locator(`[name="slots"][value="lunch"]`).Check(); err != nil {
		t.Fatalf("choose lunch: %v", err)
	}
	b.Click(t, "Add an Ingredient")
	row := b.Locator(`[name="ing_name_1"]`)
	if err := row.Fill("lasagne sheet"); err != nil {
		t.Fatalf("name the row: %v", err)
	}
	if err := row.Blur(); err != nil {
		t.Fatalf("leave the name: %v", err)
	}
	b.Sees(t, "Use lasagna sheets")
	for field, v := range map[string]string{"ing_quantity_1": "2", "ing_kcal_1": "350", "ing_protein_1": "12", "ing_carbs_1": "70", "ing_fat_1": "1"} {
		if err := b.Locator(`[name="` + field + `"]`).Fill(v); err != nil {
			t.Fatalf("fill %s: %v", field, err)
		}
	}
	for field, v := range map[string]string{"ing_unit_1": "piece", "ing_usual_1": "pantry"} {
		if _, err := b.Locator(`[name="` + field + `"]`).SelectOption(playwright.SelectOptionValues{Values: &[]string{v}}); err != nil {
			t.Fatalf("choose %s: %v", field, err)
		}
	}
	name := b.Locator(`[name="name"]`)
	if err := name.Fill("Bake"); err != nil {
		t.Fatalf("name the Recipe: %v", err)
	}
	if err := name.Press("Enter"); err != nil {
		t.Fatalf("press Enter: %v", err)
	}
	b.IsConfirmed(t, "Added Bake to the Recipe Library.")
}
