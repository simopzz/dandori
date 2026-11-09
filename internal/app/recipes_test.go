package app_test

import (
	"net/http"
	"strings"
	"testing"

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

// Each validation is refused on its field, with the reason, keeping what was typed.
func TestARecipesProblemsAreRefusedOnTheirFields(t *testing.T) {
	for _, c := range []struct {
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
	} {
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
	if !strings.Contains(unknown.Body, `name="ing_kcal_2"`) || !strings.Contains(unknown.Body, `value="120"`) {
		t.Errorf("the fields aren't numbered for the row or lost what was typed: %s", unknown.Body)
	}
}

// Without the script, the add buttons post the form back with one more row, saving
// nothing and refusing nothing.
func TestAddingARowWithoutTheScriptKeepsWhatWasTyped(t *testing.T) {
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
	active := b.Locator(`[name="active"]`)
	focused, err := active.Evaluate("el => el === document.activeElement", nil)
	if err != nil || focused != true {
		t.Errorf("the invalid field isn't focused: %v %v", focused, err)
	}
}
