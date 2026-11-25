package app_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/apptest"
)

// riceBowl is a valid Recipe of one Ingredient the User already has, rice.
func riceBowl() apptest.Recipe {
	return apptest.Recipe{
		Name: "Rice bowl", Portions: "2", Active: "10", Total: "25", Appliances: []string{"microwave", "hob"},
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
	s.Sees("Makes 2 Portions Active 10 min Total 25 min Eaten at lunch, dinner Needs hob, microwave")
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
	s.Sees("Calories (kcal) Required.")
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

// Fibre an Ingredient doesn't know makes the page write it as a floor, "3+ g", which a
// screen reader says as "at least 3 g".
func TestARecipesPageShowsFibreAsAFloorWhenAnIngredientDoesntKnowIt(t *testing.T) {
	s := kitchenWith(t, rice)
	r := riceBowl()
	r.Portions = "1"
	r.Ingredients = append(r.Ingredients, tofu())

	s.WritesRecipe(r)
	s.Sees("fibre at least 3 + g sugar – unknown")
	if body := s.Page().Body; !strings.Contains(body, `3<span aria-hidden="true">+</span>`) {
		t.Errorf("the floor isn't written 3+, the + against the number: %s", body)
	}
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

// A Recipe's kinds of appliance are shown in the User's Language, on its page and on its
// edit form, ticked.
func TestARecipesKindsOfApplianceAreShownInTheUsersLanguage(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("bea@example.com")
	s.SignsIn("bea@example.com")
	s.ChoosesLanguage("Italiano")
	r := riceBowl()
	r.Name, r.Ingredients, r.Appliances = "Tofu al forno", []apptest.RecipeRow{tofu()}, []string{"air_fryer", "oven"}

	s.WritesRecipe(r)
	s.Sees("Richiede forno, friggitrice ad aria")
	s.Follows("Modifica")
	draftHolds(t, s, map[string]string{"appliances": "oven\nair_fryer"})
	s.Sees("Elettrodomestici (facoltativi) forno piano cottura microonde friggitrice ad aria")
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
	if shown := apptest.Text(known.Body); !strings.Contains(shown, "Quantity (g)") {
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
	unit := s.HTMXGet(t, "/recipes/ingredient-row?ing_name_2=tofu&ing_new_2=true&ing_unit_2=piece")
	if shown := apptest.Text(unit.Body); !strings.Contains(shown, "Quantity (pieces)") || !strings.Contains(shown, "Per piece, as on the label.") {
		t.Errorf("a new Ingredient's unit chosen shows %q", shown)
	}
}

// In a browser, a row follows its name and a new Ingredient's unit in place: the field
// the User moved to keeps the focus, the quantity is given in the unit chosen, and the
// row says what the values are per.
func TestBrowserARowFollowsItsNewIngredientsUnit(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/recipes/new")

	if err := b.Locator(`[name="ing_name_0"]`).Fill("tofu"); err != nil {
		t.Fatalf("name the row: %v", err)
	}
	if err := b.Locator(`[name="ing_name_0"]`).Press("Tab"); err != nil {
		t.Fatalf("leave the name: %v", err)
	}
	expect := playwright.NewPlaywrightAssertions()
	unit := b.Locator(`[name="ing_unit_0"]`)
	if err := unit.WaitFor(); err != nil {
		t.Fatalf("the new Ingredient's fields never showed: %v", err)
	}
	if err := expect.Locator(b.Locator(`[name="ing_quantity_0"]`)).ToBeFocused(); err != nil {
		t.Errorf("the quantity lost the focus: %v", err)
	}
	if err := unit.Focus(); err != nil {
		t.Fatalf("focus the unit: %v", err)
	}
	if _, err := unit.SelectOption(playwright.SelectOptionValues{Values: &[]string{"ml"}}); err != nil {
		t.Fatalf("choose ml: %v", err)
	}
	if err := expect.Locator(b.Locator(".ingredient-row").First()).ToContainText("Per 100 ml, as on the label."); err != nil {
		t.Errorf("the row doesn't say what its values are per: %v", err)
	}
	if err := expect.Locator(b.Locator(`label:has([name="ing_quantity_0"])`)).ToContainText("Quantity (ml)"); err != nil {
		t.Errorf("the quantity isn't given in ml: %v", err)
	}
	if err := expect.Locator(unit).ToBeFocused(); err != nil {
		t.Errorf("the unit lost the focus: %v", err)
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

// Without htmx, the add buttons post the form back with one more row, saving nothing and
// refusing nothing.
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

// removesRow posts `form` with its row at `index` pressed Remove, as htmx and as a plain
// post, checking each comes back as `want`, saving nothing and refusing nothing.
func removesRow(t *testing.T, s *apptest.Scenario, form apptest.FormData, index string, want map[string]string, gone string) {
	t.Helper()
	form.Values.Set("remove", index)
	for _, post := range []struct {
		how string
		do  func(testing.TB, string, url.Values) apptest.Response
	}{{"as htmx", s.HTMXPostForm}, {"as a plain post", s.PostForm}} {
		page := post.do(t, form.Action, form.Values)
		if page.StatusCode != http.StatusOK {
			t.Fatalf("removing a row %s answered %d", post.how, page.StatusCode)
		}
		got := apptest.Form(t, page.Body, "recipe-form")
		for field, v := range want {
			if got.Get(field) != v {
				t.Errorf("removing a row %s left %s %q, want %q", post.how, field, got.Get(field), v)
			}
		}
		if got.Values.Has(gone) {
			t.Errorf("removing a row %s still has %s: %v", post.how, gone, got.Values)
		}
		if text := apptest.Text(page.Body); strings.Contains(text, "Give it") || strings.Contains(text, "Required") {
			t.Errorf("removing a row %s refused something: %s", post.how, text)
		}
	}
}

// A row's Remove posts the Recipe form back without that row, the rest as typed and
// numbered on from 0, saving nothing.
func TestRemovingARowKeepsTheRestAsTyped(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.Open("/recipes/new")
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	r := riceBowl()
	r.Ingredients = append(r.Ingredients, apptest.RecipeRow{Ingredient: apptest.Ingredient{Name: "egg"}, Quantity: "3"})
	r.Fill(form)

	removesRow(t, s, form, "0", map[string]string{"name": "Rice bowl", "portions": "2", "ing_name_0": "egg", "ing_quantity_0": "3"}, "ing_name_1")
	s.Open("/recipes")
	s.Sees("No Recipes yet.")
}

// A Recipe Draft's row is removed as a Recipe form's is, a new Ingredient's values moving
// up with its row.
func TestRemovingARecipeDraftsRowKeepsTheRestAsTyped(t *testing.T) {
	s := draftedLasagne(t)
	s.Follows("Recipe Draft")
	form := apptest.Form(t, s.Page().Body, "recipe-form")
	form.Values.Set("ing_quantity_1", "300")

	removesRow(t, s, form, "0", map[string]string{
		"name": "Lasagne", "ing_name_0": "lasagne sheets", "ing_quantity_0": "300", "ing_kcal_0": "350", "ing_name_3": "rice",
	}, "ing_name_4")
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

// In a browser, a row's Remove takes it off the form, the next row moving up as typed,
// and the address stays the form's.
func TestBrowserRemovesARow(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/recipes/new")
	b.Click(t, "Add an Ingredient")
	if err := b.Locator(`[name="ing_name_1"]`).Fill("tofu"); err != nil {
		t.Fatalf("name the second row: %v", err)
	}

	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Remove"}).First().Click(); err != nil {
		t.Fatalf("remove the first row: %v", err)
	}
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(b.Locator(`[name="ing_name_0"]`)).ToHaveValue("tofu"); err != nil {
		t.Errorf("the second row didn't move up: %v", err)
	}
	if err := expect.Locator(b.Locator(`[name="ing_name_1"]`)).ToHaveCount(0); err != nil {
		t.Errorf("the removed row is still there: %v", err)
	}
	b.IsAt(t, "/recipes/new")
}

// In a browser, the Library's floating + pastes a recipe first, and, Write chosen, opens
// the Recipe form.
func TestBrowserTheFloatingPlusOpensTheRecipeForm(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/recipes")
	expect := playwright.NewPlaywrightAssertions()

	if err := expect.Locator(b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Paste a recipe"})).ToBeVisible(); err != nil {
		t.Errorf("the + doesn't start on Paste: %v", err)
	}
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Choose what to add"}).Click(); err != nil {
		t.Fatalf("open what to add: %v", err)
	}
	if err := b.GetByRole("radio", playwright.PageGetByRoleOptions{Name: "Write a Recipe"}).Click(); err != nil {
		t.Fatalf("choose Write: %v", err)
	}
	if err := b.Locator(`a.fab[aria-label="Write a Recipe"]`).Click(); err != nil {
		t.Fatalf("press the +: %v", err)
	}
	b.IsAt(t, "/recipes/new")
}

// In a browser, an empty Recipe form is refused by the app, not the browser: its errors
// are in the User's Language, Italian on an English browser, each refused field marked.
func TestBrowserAnEmptyRecipeShowsTheAppsErrorsInTheUsersLanguage(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.ChooseLanguage(t, "Italiano", "Continue")
	b.IsAt(t, "/")
	b.Open(t, "/recipes/new")

	b.Click(t, "Salva la Ricetta")
	b.Sees(t, "Dalle un nome.")
	b.Sees(t, "Scegli almeno un tipo di Pasto.")
	for _, field := range []string{"name", "portions", "active", "total", "slots", "ing_name_0"} {
		if err := playwright.NewPlaywrightAssertions().Locator(b.Locator(`[name="`+field+`"]`).First()).ToHaveAttribute("aria-invalid", "true"); err != nil {
			t.Errorf("the refused %s isn't marked: %v", field, err)
		}
	}
}

// The Recipe form says its Steps and Appliances may be left blank, in each Language.
func TestTheRecipeFormSaysWhatIsOptional(t *testing.T) {
	for language, want := range map[string][]string{
		"English":  {"Steps (optional)", "Appliances (optional)"},
		"Italiano": {"Passaggi (facoltativi)", "Elettrodomestici (facoltativi)"},
	} {
		t.Run(language, func(t *testing.T) {
			s := kitchenIn(t, language)
			s.Open("/recipes/new")
			for _, w := range want {
				s.Sees(w)
			}
		})
	}
}

// In a browser, a rejected Recipe form opens on its error summary, whose first link takes
// focus to its field, the first refused.
func TestBrowserARejectedRecipeOpensOnItsErrorSummary(t *testing.T) {
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
	summary := b.Locator(".error-summary")
	if err := playwright.NewPlaywrightAssertions().Locator(summary).ToBeFocused(); err != nil {
		t.Fatalf("the error summary isn't focused: %v", err)
	}
	first := summary.GetByRole("link").First()
	if err := playwright.NewPlaywrightAssertions().Locator(first).ToHaveText("Active time: Can't be more than the total minutes."); err != nil {
		t.Errorf("the summary's first link: %v", err)
	}
	if err := first.Click(); err != nil {
		t.Fatalf("follow the summary's first link: %v", err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(b.Locator(`[name="active"]`)).ToBeFocused(); err != nil {
		t.Errorf("the link doesn't focus its field: %v", err)
	}
}

// In a browser, a Recipe with no Ingredient is summed up by a link to the first row.
func TestBrowserARecipeWithNoIngredientLinksToTheFirstRow(t *testing.T) {
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
	if err := b.Locator(".error-summary").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredient: List at least one Ingredient."}).Click(); err != nil {
		t.Fatalf("follow the summary's link: %v", err)
	}
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
	s.Sees("Makes 2 Portions Active 10 min Total 25 min Eaten at lunch, dinner Needs hob, microwave")
	s.Sees("rice 200 g")
	s.Sees("Rinse the rice. Cook it.")

	s.Follows("Edit")
	r := riceBowl()
	r.Name, r.Portions, r.Slots, r.Steps = "Rice pot", "4", []string{"dinner"}, []string{"Boil it."}
	r.Appliances = []string{"oven"}
	r.Ingredients[0].Quantity = "400"
	s.PostsRecipeForm(r)
	s.IsAt(page)
	s.Sees("Rice pot")
	s.Sees("Makes 4 Portions Active 10 min Total 25 min Eaten at dinner Needs oven")
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

// Deleting a Recipe asks once, naming it, then removes it and frees its Ingredients for
// deleting or changing unit. The edit form has no Delete that posts at once.
func TestDeletingARecipeRemovesItAndFreesItsIngredients(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	page := s.Page().Path

	s.Follows("Edit")
	if strings.Contains(s.Page().Body, `action="`+page+`/delete"`) {
		t.Errorf("the edit form deletes the Recipe in one post:\n%s", s.Page().Body)
	}
	s.Follows("Delete")
	s.IsAt(page + "/delete")
	s.Sees("Delete the Recipe Rice bowl?")
	s.Submits("remove-step")
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

// The sheet a Recipe's Delete opens asks in the User's Language, and its form posted as
// htmx lands on the Library confirming it, as a plain post does.
func TestARecipeIsDeletedFromItsSheet(t *testing.T) {
	s := kitchenIn(t, "Italiano", rice)
	s.WritesRecipe(riceBowl())
	s.Open(s.Page().Path + "/edit")
	sheet := s.HTMXGet(t, apptest.SheetSource(t, s.Page().Body, "Elimina"))
	if shown := apptest.Text(sheet.Body); sheet.StatusCode != http.StatusOK || strings.Contains(sheet.Body, "<html") || !strings.Contains(shown, "Eliminare la Ricetta Rice bowl?") {
		t.Fatalf("the sheet answered %d:\n%s", sheet.StatusCode, sheet.Body)
	}
	step := apptest.Form(t, sheet.Body, "remove-step")
	got := s.HTMXPostForm(t, step.Action, step.Values)
	if got.Path != "/recipes" {
		t.Errorf("deleting from the sheet landed on %s", got.Path)
	}
	if notices := apptest.Notices(t, got.Body); !slices.Equal(notices, []string{"Eliminata: Rice bowl."}) {
		t.Errorf("deleting from the sheet confirms %q", notices)
	}
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
	for _, step := range []string{page + "/delete", page + "/delete/sheet"} {
		if got := s.Get(t, step); got.StatusCode != http.StatusNotFound {
			t.Errorf("the step deleting another User's Recipe, %s, answered %d", step, got.StatusCode)
		}
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
// the edit form's Delete opens a sheet asking first: closed, it leaves the Recipe, and
// confirmed, it lands on the Library saying so.
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
	b.Sees(t, "Edit a Recipe")
	edit := b.URL()
	deleteLink := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Delete", Exact: playwright.Bool(true)})
	sheet := b.Locator("#recipe-remove")
	if err := deleteLink.Click(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(sheet).ToContainText("Delete the Recipe Lentil soup?"); err != nil {
		t.Fatalf("the sheet asking first never showed: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click(); err != nil {
		t.Fatalf("close the sheet: %v", err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(sheet).Not().ToBeVisible(); err != nil {
		t.Errorf("the sheet stayed open: %v", err)
	}
	if got := b.URL(); got != edit {
		t.Errorf("closing the sheet left the edit form for %s", got)
	}
	if err := deleteLink.Click(); err != nil {
		t.Fatalf("delete again: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"}).Click(); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	b.IsAt(t, "/recipes")
	b.IsConfirmed(t, "Deleted Lentil soup.")
	b.Sees(t, "No Recipes yet.")
}

// The Recipe Library comes before any form: the page holds no paste form, and its + opens
// one in a sheet, or without htmx the paste page. Pasted from either, a recipe starts a
// Run; a paste refused lands on the paste page, with why.
func TestTheRecipeLibraryComesBeforeThePasteForm(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	s.Open("/recipes")
	s.Sees("Rice bowl")
	library := s.Page().Body
	for _, form := range []string{`id="paste-form"`, `id="recipe-form"`} {
		if strings.Contains(library, form) {
			t.Errorf("the Recipe Library holds the form %s", form)
		}
	}
	for _, href := range []string{`href="/recipes/paste"`, `href="/recipes/new"`} {
		if !strings.Contains(library, href) {
			t.Errorf("the Recipe Library's + has no link %s", href)
		}
	}

	s.Model.Script(fakellm.Reply("Not a recipe."), fakellm.Reply("Not a recipe."))
	sheet := s.HTMXGet(t, "/recipes/paste/sheet")
	if sheet.StatusCode != http.StatusOK || strings.Contains(sheet.Body, "<html") {
		t.Fatalf("the paste sheet's body answered %d, want 200 and a fragment:\n%s", sheet.StatusCode, sheet.Body)
	}
	form := apptest.Form(t, sheet.Body, "paste-form")
	form.Values.Set("text", "Two eggs, boiled.")
	if ran := s.PostForm(t, form.Action, form.Values); !strings.HasPrefix(ran.Path, "/runs/") {
		t.Errorf("pasting from the sheet lands on %s, want its Run", ran.Path)
	}

	s.StartsParse("Two eggs, fried.")
	if !strings.HasPrefix(s.Page().Path, "/runs/") {
		t.Errorf("pasting from the paste page lands on %s, want its Run", s.Page().Path)
	}

	s.StartsParse("  ")
	s.IsRefused()
	s.Sees("Paste a recipe to read.")
	apptest.Form(t, s.Page().Body, "paste-form")
}

// On a phone, the first Recipe in the Recipe Library shows without scrolling.
func TestBrowserTheFirstRecipeShowsOnAPhoneWithoutScrolling(t *testing.T) {
	s := kitchenWith(t, rice)
	s.WritesRecipe(riceBowl())
	b := s.Browser(t)
	if err := b.SetViewportSize(360, 640); err != nil {
		t.Fatalf("size a phone: %v", err)
	}
	b.SignIn(t, "ada@example.com")
	b.IsAt(t, "/")
	b.Open(t, "/recipes")
	first := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Rice bowl"})
	if err := playwright.NewPlaywrightAssertions().Locator(first).ToBeInViewport(); err != nil {
		t.Errorf("the first Recipe is below the fold on a phone: %v", err)
	}
}

// Without JS, the Recipe Library's + reaches the paste form, and the Ingredients page's
// the add form, each on a page of its own.
func TestBrowserTheAddButtonsReachTheirFormsWithoutJS(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.BrowserWithoutJS(t)
	b.SignUp(t, "ada@example.com")
	expect := playwright.NewPlaywrightAssertions()

	b.Open(t, "/recipes")
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Paste a recipe"}).Click(); err != nil {
		t.Fatalf("press Paste: %v", err)
	}
	b.IsAt(t, "/recipes/paste")
	if err := expect.Locator(b.GetByLabel("Paste a recipe")).ToBeVisible(); err != nil {
		t.Errorf("Paste doesn't reach the paste form: %v", err)
	}

	b.Open(t, "/ingredients")
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Add an Ingredient"}).Click(); err != nil {
		t.Fatalf("press Add: %v", err)
	}
	b.IsAt(t, "/ingredients/new")
	if err := expect.Locator(b.Locator("#ingredient-add").GetByLabel("Name")).ToBeVisible(); err != nil {
		t.Errorf("Add doesn't reach the add form: %v", err)
	}
}
