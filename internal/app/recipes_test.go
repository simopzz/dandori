package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// riceBowl is a valid Recipe of one Ingredient the User already has, rice.
func riceBowl() apptest.Recipe {
	return apptest.Recipe{
		Name: "Rice bowl", Portions: "2", Active: "10", Total: "25", Appliances: "rice cooker\nknife",
		Slots: []string{"lunch", "dinner"}, Steps: []string{"Rinse the rice.", "Cook it."},
		Ingredients: []apptest.RecipeRow{{Ingredient: apptest.Ingredient{Name: "rice"}, Quantity: "200"}},
	}
}

// A Recipe written by hand appears in the Recipe Library, and its page shows all of it.
// 200 g of rice over two Portions is 350 kcal each, and nothing knows its fibre or sugar.
func TestARecipeWrittenByHandAppearsInTheRecipeLibrary(t *testing.T) {
	s := kitchenWith(t, rice)

	s.Open("/recipes")
	s.Sees("No Recipes yet.")
	s.WritesRecipe(riceBowl())
	s.IsConfirmed("Added Rice bowl to the Recipe Library.")
	s.Sees("Rice bowl")
	s.Sees("Makes 2 Portions Active 10 min Total 25 min Eaten at lunch, dinner Needs rice cooker, knife")
	s.Sees("350 kcal protein 7 g carbs 78 g fat 1 g fibre – unknown sugar – unknown")
	s.Sees("rice 200 g")
	s.Sees("Rinse the rice. Cook it.")

	s.Open("/recipes")
	s.Sees("Rice bowl lunch, dinner 25 min 350 kcal per Portion")
	s.DoesNotSee("No Recipes yet.")
}

// The app's navigation reaches the Recipe Library.
func TestTheNavigationReachesTheRecipeLibrary(t *testing.T) {
	s := kitchenWith(t)
	s.Open("/kitchen")
	s.Follows("Recipes")
	s.IsAt("/recipes")
}

// tofu is a row for a new Ingredient, its fields confirmed.
func tofu() apptest.RecipeRow {
	return apptest.RecipeRow{
		Ingredient: apptest.Ingredient{Name: "tofu", Unit: "g", Usual: "fridge", Kcal: "120", Protein: "12", Carbs: "2", Fat: "7", Fibre: "1"},
		Quantity:   "300", Confirmed: true,
	}
}

// A name no Ingredient has opens its row to confirm, saving nothing; confirmed, the
// Ingredient is created with the Recipe.
func TestANewIngredientIsConfirmedInTheFormAndCreatedWithTheRecipe(t *testing.T) {
	s := kitchenWith(t, rice)
	r := riceBowl()
	r.Ingredients = append(r.Ingredients, apptest.RecipeRow{Ingredient: apptest.Ingredient{Name: "tofu"}, Quantity: "300"})

	s.WritesRecipe(r)
	s.IsRefused()
	s.Sees("Some Ingredients are new: check their values below, then save.")
	s.DoesNotSee("Required.")
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	if got := form.Get("ing_quantity_1"); got != "300" {
		t.Errorf("the row kept the quantity %q", got)
	}
	s.Open("/recipes")
	s.Sees("No Recipes yet.")

	s.Open("/recipes/new")
	r.Ingredients[1] = tofu()
	s.PostsRecipeForm(r)
	s.Sees("tofu 300 g")
	s.Open("/ingredients")
	s.Sees("tofu")
}

// A new Ingredient's fields are refused on theirs.
func TestANewIngredientsMissingValuesAreRefusedOnTheirFields(t *testing.T) {
	s := kitchenWith(t, rice)
	r := riceBowl()
	row := tofu()
	row.Kcal, row.Usual = "", ""
	r.Ingredients = []apptest.RecipeRow{row}

	s.WritesRecipe(r)
	s.IsRefused()
	s.Sees("Calories Required.")
	s.Sees("Choose where it is usually kept.")
	s.Open("/ingredients")
	s.DoesNotSee("tofu")
}

// A failing Recipe creates neither it nor its new Ingredient.
func TestARecipeThatFailsCreatesNoIngredient(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())

	r := riceBowl()
	r.Ingredients = append(r.Ingredients, tofu())
	s.WritesRecipe(r)
	s.IsRefused()
	s.Open("/ingredients")
	s.DoesNotSee("tofu")
}

// recipeRefusals are the ways the Recipe form refuses a Recipe, writing or editing it.
var recipeRefusals = []struct {
	name    string
	change  func(*apptest.Recipe)
	refused string
}{
	{"no name", func(r *apptest.Recipe) { r.Name = " " }, "Give it a name."},
	{"no Ingredient", func(r *apptest.Recipe) { r.Ingredients = nil }, "List at least one Ingredient."},
	{"an Ingredient twice", func(r *apptest.Recipe) { r.Ingredients = append(r.Ingredients, r.Ingredients[0]) }, "Already listed in this Recipe."},
	{"no quantity", func(r *apptest.Recipe) { r.Ingredients[0].Quantity = "" }, "Say how much."},
	{"a quantity of 0", func(r *apptest.Recipe) { r.Ingredients[0].Quantity = "0" }, "Must be more than 0."},
	{"half a gram", func(r *apptest.Recipe) { r.Ingredients[0].Quantity = "0.5" }, "Whole grams and millilitres only."},
	{"no kind of Slot", func(r *apptest.Recipe) { r.Slots = nil }, "Choose at least one kind of Slot."},
	{"no Portions", func(r *apptest.Recipe) { r.Portions = "0" }, "Must be a whole number of at least 1."},
	{"no total minutes", func(r *apptest.Recipe) { r.Total, r.Active = "0", "0" }, "Must be more than 0."},
	{"blank active minutes", func(r *apptest.Recipe) { r.Active = "" }, "Say how many minutes."},
	{"active above total", func(r *apptest.Recipe) { r.Active = "26" }, "Can't be more than the total minutes."},
	{"over a week", func(r *apptest.Recipe) { r.Total = "10081" }, "Must be at most 10080 minutes."},
}

// Each validation is refused on its field, with the reason, keeping what was typed.
func TestARecipesProblemsAreRefusedOnTheirFields(t *testing.T) {
	for _, c := range recipeRefusals {
		t.Run(c.name, func(t *testing.T) {
			s := kitchenWith(t, rice)
			r := riceBowl()
			c.change(&r)
			s.WritesRecipe(r)
			s.IsRefused()
			s.Sees(c.refused)
			if got := apptest.Form(t, s.Page().Body, "recipe-form").Get("portions"); got != r.Portions {
				t.Errorf("the refused form holds %q Portions, typed %q", got, r.Portions)
			}
			s.Open("/recipes")
			s.Sees("No Recipes yet.")
		})
	}
}

// A second Recipe of the same name, ignoring case and surrounding space, is refused with
// a link to the existing one.
func TestASecondRecipeOfTheSameNameIsRefusedWithALinkToTheFirst(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	first := s.Page().Path

	r := riceBowl()
	r.Name = "  RICE BOWL "
	s.WritesRecipe(r)
	s.IsRefused()
	s.Sees("You already have a Recipe with this name: Rice bowl")
	s.Follows("Rice bowl")
	s.IsAt(first)
}

// Fibre an Ingredient doesn't know makes the page say "at least".
func TestARecipesPageShowsFibreAsAFloorWhenAnIngredientDoesntKnowIt(t *testing.T) {
	s := kitchenWith(t, rice)
	r := riceBowl()
	r.Portions = "1"
	r.Ingredients = append(r.Ingredients, tofu())

	s.WritesRecipe(r)
	s.Sees("fibre at least 3 g sugar – unknown")
}

// A Recipe fixes the User's Language, like any Food Term.
func TestARecipeFixesTheLanguage(t *testing.T) {
	s := kitchenWith(t)
	r := riceBowl()
	r.Ingredients = []apptest.RecipeRow{tofu()}
	s.WritesRecipe(r)

	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.IsRefused()
}

// An Ingredient a Recipe lists refuses deletion and a change of unit, naming the Recipe.
func TestAnIngredientInARecipeRefusesDeletionAndAChangeOfUnit(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())

	s.Open("/ingredients")
	s.DeletesIngredient("rice")
	s.IsRefused()
	s.Sees("Used by Rice bowl, so it can't be deleted.")

	s.Open("/ingredients")
	changed := rice
	changed.Unit = "ml"
	s.EditsIngredient("rice", changed)
	s.IsRefused()
	s.Sees("Used by Rice bowl, so its unit stays.")

	s.Open("/ingredients")
	s.EditsIngredient("rice", rice)
	s.IsAt("/ingredients")
}

// More than one Recipe listing it are counted after the first by name.
func TestAnIngredientInSeveralRecipesCountsTheOthers(t *testing.T) {
	s := kitchenWith(t, rice)
	for _, name := range []string{"Rice bowl", "Arancini", "Risotto"} {
		r := riceBowl()
		r.Name = name
		s.WritesRecipe(r)
	}
	s.Open("/ingredients")
	s.DeletesIngredient("rice")
	s.IsRefused()
	s.Sees("Used by Arancini and 2 more Recipes, so it can't be deleted.")
}

// Two Users never see each other's Recipes, even by address.
func TestTwoUsersNeverSeeEachOthersRecipes(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	page := s.Page().Path

	s.Invite("grace@example.com")
	s.SwitchesTo("grace@example.com")
	s.ChoosesLanguage("English")
	s.Open("/recipes")
	s.DoesNotSee("Rice bowl")
	if got := s.Get(t, page); got.StatusCode != http.StatusNotFound {
		t.Errorf("another User's Recipe answered %d", got.StatusCode)
	}
}

// The row's name says what it is as the User leaves it: htmx asks for the known unit or
// the fields to confirm a new one.
func TestARowsNameShowsTheKnownUnitOrTheNewIngredientsFields(t *testing.T) {
	s := kitchenWith(t, rice)

	known := s.HTMXGet(t, "/recipes/ingredient-row?ing_name_2=Rice")
	if shown := apptest.Text(known.Body); !strings.Contains(shown, "Measured in g") {
		t.Errorf("a known name shows %q", shown)
	}
	unknown := s.HTMXGet(t, "/recipes/ingredient-row?ing_name_2=tofu&ing_kcal_2=120")
	if shown := apptest.Text(unknown.Body); !strings.Contains(shown, "Calories") || strings.Contains(shown, "Required") {
		t.Errorf("a new name shows %q", shown)
	}
	renamed := s.HTMXGet(t, "/recipes/ingredient-row?ing_name_2=seitan&ing_new_2=true")
	if shown := apptest.Text(renamed.Body); strings.Contains(shown, "Required") {
		t.Errorf("renaming a new row refuses its blank values before any save: %q", shown)
	}
	if !strings.Contains(unknown.Body, `name="ing_kcal_2"`) || !strings.Contains(unknown.Body, `value="120"`) {
		t.Errorf("the fields aren't numbered for the row or lost what was typed: %s", unknown.Body)
	}
}

// htmx asks the server for one more row, numbered after those the form has.
func TestABlankRowIsNumberedAfterTheFormsRows(t *testing.T) {
	s := kitchenWith(t, rice)

	row := s.HTMXGet(t, "/recipes/ingredient-row/blank?ing_name_0=Rice&ing_name_1=")
	if !strings.Contains(row.Body, `name="ing_name_2"`) || strings.Contains(row.Body, "<html") {
		t.Errorf("the blank row came back as %s", row.Body)
	}
}

// Without htmx, the add buttons post the form back with one more row, saving
// nothing and refusing nothing.
func TestAddingARowWithoutHTMXKeepsWhatWasTyped(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Open("/recipes/new")
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	form.Values.Set("name", "Rice bowl")
	form.Values.Set("add", "ingredients")

	page := s.PostForm(t, form.Action, form.Values)
	if page.StatusCode != http.StatusOK {
		t.Fatalf("adding a row answered %d", page.StatusCode)
	}
	got := apptest.Form(t, page.Body, "recipe-form")
	if !got.Values.Has("ing_name_1") || got.Get("name") != "Rice bowl" {
		t.Errorf("the form came back as %v", got.Values)
	}
	if strings.Contains(apptest.Text(page.Body), "Give it") {
		t.Errorf("adding a row refused something: %s", apptest.Text(page.Body))
	}
}

// In a browser, adding a row gives one more, numbered, whose name asks what it is.
func TestBrowserAddsAnIngredientRowAndConfirmsANewOne(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/recipes/new")

	b.Click(t, "Add an Ingredient")
	field := b.Locator(`[name="ing_name_1"]`)
	if err := field.Fill("tofu"); err != nil {
		t.Fatalf("name the row: %v", err)
	}
	if err := field.Blur(); err != nil {
		t.Fatalf("leave the name: %v", err)
	}
	if err := b.Locator(`[name="ing_kcal_1"]`).WaitFor(); err != nil {
		t.Fatalf("the new Ingredient's fields never showed: %v", err)
	}
	b.Shot(t, "recipe-form")
}

// In a browser, each add gives one more row, numbered after the last, with focus in it.
func TestBrowserEachAddGivesANumberedRowAndFocusesIt(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/recipes/new")

	focused := func(selector string) {
		t.Helper()
		if err := playwright.NewPlaywrightAssertions().Locator(b.Locator(selector)).ToBeFocused(); err != nil {
			t.Errorf("%s isn't focused: %v", selector, err)
		}
	}
	b.Click(t, "Add an Ingredient")
	focused(`[name="ing_name_1"]`)
	b.Click(t, "Add an Ingredient")
	focused(`[name="ing_name_2"]`)
	b.Click(t, "Add a step")
	focused(`#steps li:nth-child(2) textarea`)
}

// In a browser, the Library's floating + opens the Recipe form.
func TestBrowserTheFloatingPlusOpensTheRecipeForm(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/recipes")

	if err := b.Locator(`a.fab[aria-label="Write a Recipe"]`).Click(); err != nil {
		t.Fatalf("press the +: %v", err)
	}
	b.IsAt(t, "/recipes/new")
}

// In a browser, a rejected form comes back with focus on the invalid field.
func TestBrowserARejectedRecipeFocusesItsInvalidField(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/recipes/new")

	for field, v := range map[string]string{"name": "Bowl", "portions": "2", "active": "30", "total": "10"} {
		if err := b.Locator(`[name="` + field + `"]`).Fill(v); err != nil {
			t.Fatalf("fill %s: %v", field, err)
		}
	}
	b.Click(t, "Save Recipe")
	b.Sees(t, "Can't be more than the total minutes.")
	if err := playwright.NewPlaywrightAssertions().Locator(b.Locator(`[name="active"]`)).ToBeFocused(); err != nil {
		t.Errorf("the invalid field isn't focused: %v", err)
	}
}

// In a browser, a Recipe with no Ingredient comes back focused on the first row.
func TestBrowserARecipeWithNoIngredientFocusesTheFirstRow(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/recipes/new")

	for field, v := range map[string]string{"name": "Bowl", "portions": "2", "active": "5", "total": "10"} {
		if err := b.Locator(`[name="` + field + `"]`).Fill(v); err != nil {
			t.Fatalf("fill %s: %v", field, err)
		}
	}
	if err := b.GetByLabel("lunch").Check(); err != nil {
		t.Fatalf("choose lunch: %v", err)
	}
	b.Click(t, "Save Recipe")
	b.Sees(t, "List at least one Ingredient.")
	if err := playwright.NewPlaywrightAssertions().Locator(b.Locator(`[name="ing_name_0"]`)).ToBeFocused(); err != nil {
		t.Errorf("the first row isn't focused: %v", err)
	}
}

// The edit form holds the Recipe as it is, so saving it untouched changes nothing, and
// what it changes shows on the Recipe's page and in the Library.
func TestEditingARecipeChangesItsPageAndTheLibrary(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	page := s.Page().Path

	s.Follows("Edit")
	s.Submits("recipe-form")
	s.IsConfirmed("Saved Rice bowl.")
	s.IsAt(page)
	s.Sees("Makes 2 Portions Active 10 min Total 25 min Eaten at lunch, dinner Needs rice cooker, knife")
	s.Sees("rice 200 g")
	s.Sees("Rinse the rice. Cook it.")

	s.Follows("Edit")
	r := riceBowl()
	r.Name, r.Portions, r.Slots, r.Steps = "Rice pot", "4", []string{"dinner"}, []string{"Boil it."}
	r.Ingredients[0].Quantity = "400"
	s.PostsRecipeForm(r)
	s.IsAt(page)
	s.Sees("Rice pot")
	s.Sees("Makes 4 Portions")
	s.Sees("rice 400 g")
	s.DoesNotSee("Rinse the rice.")

	s.Open("/recipes")
	s.Sees("Rice pot dinner 25 min 350 kcal per Portion")
	s.DoesNotSee("Rice bowl")
}

// An edit is refused for the same reasons as writing a Recipe, on the edit form, and
// leaves the Recipe as it was.
func TestAnEditIsRefusedAsWritingARecipeIs(t *testing.T) {
	for _, c := range recipeRefusals {
		t.Run(c.name, func(t *testing.T) {
			s := kitchenWith(t, rice)
			s.WritesRecipe(riceBowl())
			page := s.Page().Path
			s.Follows("Edit")

			r := riceBowl()
			r.Portions = "3"
			c.change(&r)
			s.PostsRecipeForm(r)
			s.IsRefused()
			s.Sees(c.refused)
			if got := apptest.Form(t, s.Page().Body, "recipe-form"); got.Action != page || got.Get("portions") != r.Portions {
				t.Errorf("the refused form posts to %q holding %q Portions", got.Action, got.Get("portions"))
			}
			s.Open(page)
			s.Sees("Makes 2 Portions")
		})
	}
}

// A new Ingredient added during an edit is confirmed in the form, then created with it.
func TestANewIngredientAddedDuringAnEditIsConfirmedInTheForm(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	page := s.Page().Path
	s.Follows("Edit")

	r := riceBowl()
	r.Ingredients = append(r.Ingredients, apptest.RecipeRow{Ingredient: apptest.Ingredient{Name: "tofu"}, Quantity: "300"})
	s.PostsRecipeForm(r)
	s.IsRefused()
	s.Sees("Some Ingredients are new: check their values below, then save.")
	s.DoesNotSee("Required.")

	r.Ingredients[1] = tofu()
	s.PostsRecipeForm(r)
	s.IsAt(page)
	s.Sees("tofu 300 g")
	s.Open("/ingredients")
	s.Sees("tofu")
}

// Renaming a Recipe to another of the User's is refused with a link to it, while changing
// only how its own name is written is not.
func TestRenamingARecipeToAnotherIsRefusedWithALinkToIt(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	first := s.Page().Path
	r := riceBowl()
	r.Name = "Risotto"
	s.WritesRecipe(r)
	second := s.Page().Path

	s.Follows("Edit")
	r.Name = "  RICE BOWL "
	s.PostsRecipeForm(r)
	s.IsRefused()
	s.Sees("You already have a Recipe with this name: Rice bowl")
	s.Follows("Rice bowl")
	s.IsAt(first)

	s.Open(second + "/edit")
	r.Name = "RISOTTO"
	s.PostsRecipeForm(r)
	s.IsAt(second)
	s.Sees("RISOTTO")
}

// Deleting a Recipe removes it and frees its Ingredients for deleting or changing unit.
func TestDeletingARecipeRemovesItAndFreesItsIngredients(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	page := s.Page().Path

	s.Follows("Edit")
	s.Submits("recipe-delete")
	s.IsConfirmed("Deleted Rice bowl.")
	s.IsAt("/recipes")
	s.Sees("No Recipes yet.")
	if got := s.Get(t, page); got.StatusCode != http.StatusNotFound {
		t.Errorf("a deleted Recipe answered %d", got.StatusCode)
	}

	s.Open("/ingredients")
	changed := rice
	changed.Unit = "ml"
	s.EditsIngredient("rice", changed)
	s.IsAt("/ingredients")
	s.Open("/ingredients")
	s.DeletesIngredient("rice")
	s.IsConfirmed("Deleted rice.")
}

// An Ingredient an edit takes off the Recipe is free again.
func TestAnIngredientTakenOffARecipeIsFreed(t *testing.T) {
	s := kitchenWith(t, rice)
	r := riceBowl()
	r.Ingredients = append(r.Ingredients, tofu())
	s.WritesRecipe(r)

	s.Follows("Edit")
	r.Ingredients = r.Ingredients[:1]
	s.PostsRecipeForm(r)
	s.DoesNotSee("tofu")

	s.Open("/ingredients")
	s.DeletesIngredient("tofu")
	s.IsConfirmed("Deleted tofu.")
}

// Another User's Recipe can be neither edited nor deleted, even by address.
func TestAnotherUsersRecipeCanNeitherBeEditedNorDeleted(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	page := s.Page().Path
	s.Follows("Edit")
	form := apptest.Form(t, s.Page().Body, "recipe-form")

	s.Invite("grace@example.com")
	s.SwitchesTo("grace@example.com")
	s.ChoosesLanguage("English")
	if got := s.Get(t, page+"/edit"); got.StatusCode != http.StatusNotFound {
		t.Errorf("the edit form of another User's Recipe answered %d", got.StatusCode)
	}
	s.Open("/ingredients")
	s.AddsIngredient(rice)
	if got := s.PostForm(t, form.Action, form.Values); got.StatusCode != http.StatusNotFound {
		t.Errorf("editing another User's Recipe answered %d", got.StatusCode)
	}
	if got := s.PostForm(t, page+"/delete", nil); got.StatusCode != http.StatusNotFound {
		t.Errorf("deleting another User's Recipe answered %d", got.StatusCode)
	}
}

// The Library's search finds a Recipe by name, through a typo, and offers itself only
// once there is something to search.
func TestTheRecipeLibrarysSearchFindsARecipeThroughATypo(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Open("/recipes")
	if strings.Contains(s.Page().Body, `id="recipe-search"`) {
		t.Error("the search shows with no Recipe to find")
	}
	for _, name := range []string{"Rice bowl", "Lentil soup", "Arancini"} {
		r := riceBowl()
		r.Name = name
		s.WritesRecipe(r)
	}

	s.Open("/recipes")
	s.SearchesRecipes("lentil")
	s.Sees("Lentil soup")
	s.DoesNotSee("Rice bowl")
	if got := apptest.Form(t, s.Page().Body, "recipe-search").Get("q"); got != "lentil" {
		t.Errorf("the search holds %q", got)
	}

	s.SearchesRecipes("lentl soop")
	s.Sees("Lentil soup")
	s.DoesNotSee("Arancini")

	s.SearchesRecipes("zzzz")
	s.Sees("No Recipe is like zzzz.")

	s.SearchesRecipes("")
	s.Sees("Arancini")
	s.Sees("Rice bowl")

	list := s.HTMXGet(t, "/recipes/list?q=lentl")
	if shown := apptest.Text(list.Body); !strings.Contains(shown, "Lentil soup") || strings.Contains(shown, "Arancini") || strings.Contains(list.Body, "<html") {
		t.Errorf("the search swapped in %s", list.Body)
	}
}

// In a browser, the Library's search swaps the list as the User types, keeping focus, and
// the edit form's Delete, a button of the form beside it, lands on the Library.
func TestBrowserSearchesTheLibraryAndDeletesARecipe(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/recipes/new")
	fill := func(values map[string]string) {
		t.Helper()
		for field, v := range values {
			if err := b.Locator(`[name="` + field + `"]`).Fill(v); err != nil {
				t.Fatalf("fill %s: %v", field, err)
			}
		}
	}
	fill(map[string]string{"name": "Lentil soup", "portions": "2", "active": "5", "total": "10", "ing_quantity_0": "300"})
	fill(map[string]string{"ing_name_0": "tofu"})
	if err := b.Locator(`[name="ing_name_0"]`).Blur(); err != nil {
		t.Fatalf("leave the name: %v", err)
	}
	if err := b.Locator(`[name="ing_kcal_0"]`).WaitFor(); err != nil {
		t.Fatalf("the new Ingredient's fields never showed: %v", err)
	}
	if err := b.GetByLabel("lunch").Check(); err != nil {
		t.Fatalf("choose lunch: %v", err)
	}
	fill(map[string]string{"ing_kcal_0": "120", "ing_protein_0": "12", "ing_carbs_0": "2", "ing_fat_0": "7"})
	for field, v := range map[string]string{"ing_unit_0": "g", "ing_usual_0": "fridge"} {
		if _, err := b.Locator(`[name="` + field + `"]`).SelectOption(playwright.SelectOptionValues{Values: &[]string{v}}); err != nil {
			t.Fatalf("choose %s: %v", field, err)
		}
	}
	b.Click(t, "Save Recipe")
	b.Sees(t, "tofu 300 g")

	b.Open(t, "/recipes")
	search := b.Locator(`#recipe-search input[name="q"]`)
	if err := search.PressSequentially("zzzz"); err != nil {
		t.Fatalf("search: %v", err)
	}
	b.Sees(t, "No Recipe is like zzzz.")
	if err := playwright.NewPlaywrightAssertions().Locator(search).ToBeFocused(); err != nil {
		t.Errorf("the search lost focus: %v", err)
	}
	if err := search.Fill("lentl"); err != nil {
		t.Fatalf("search again: %v", err)
	}
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Lentil soup"}).Click(); err != nil {
		t.Fatalf("open the Recipe found: %v", err)
	}
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Edit"}).Click(); err != nil {
		t.Fatalf("open the edit form: %v", err)
	}
	b.Click(t, "Delete")
	b.Sees(t, "No Recipes yet.")
	b.IsAt(t, "/recipes")
}
