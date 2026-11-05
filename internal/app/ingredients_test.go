package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// egg is an Ingredient whose fibre and sugar the User couldn't find.
var egg = apptest.Ingredient{
	Name: "egg", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6.3", Carbs: "0.4", Fat: "4.8",
}

// An Ingredient is added with its unit, its nutritional values per that unit and its
// usual Storage Location. Fibre and sugar left blank show as unknown, never 0, which is
// a value like any other; Staples are marked.
func TestAddingIngredients(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.Sees("No Ingredients yet.")

	s.AddsIngredient(egg)
	s.IsAt("/ingredients")
	s.Sees("Added egg.")
	s.Sees("egg fridge per piece: 70 kcal protein 6.3 g carbs 0.4 g fat 4.8 g fibre – unknown sugar – unknown")
	s.DoesNotSee("No Ingredients yet.")

	s.AddsIngredient(apptest.Ingredient{
		Name: "Rolled oats", Unit: "g", Usual: "pantry", Kcal: "372", Protein: "13.5", Carbs: "58.7", Fat: "7",
		Fibre: "10.1", Sugar: "0",
	})
	s.AddsIngredient(apptest.Ingredient{
		Name: "olive oil", Unit: "ml", Usual: "pantry", Kcal: "824", Protein: "0", Carbs: "0", Fat: "91.6", Staple: true,
	})
	s.Sees("Added olive oil.")
	s.Sees("egg fridge per piece: 70 kcal")
	s.Sees("olive oil Staple pantry per 100 ml: 824 kcal protein 0 g carbs 0 g fat 91.6 g fibre – unknown sugar – unknown")
	s.Sees("Rolled oats pantry per 100 g: 372 kcal protein 13.5 g carbs 58.7 g fat 7 g fibre 10.1 g sugar 0 g")
	shown, last := apptest.Text(s.Page().Body), -1
	for _, name := range []string{"egg fridge", "olive oil Staple", "Rolled oats pantry"} {
		at := strings.Index(shown, name)
		if at < 0 {
			t.Fatalf("the Ingredients don't show %q: %s", name, shown)
		}
		if at < last {
			t.Errorf("%q is listed out of name order: %s", name, shown)
		}
		last = at
	}
}

// Calories, protein, carbs and fat are required, and a value is a number not below 0, to
// one decimal place; a refused add keeps what was typed, with why on each field.
func TestAnIngredientNeedsItsFourNutritionalValues(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")

	s.AddsIngredient(apptest.Ingredient{Name: "egg", Kcal: "70", Protein: "-1", Carbs: "0.45", Fibre: "lots"})
	s.IsRefused()
	s.Sees("Choose g, ml or piece.")
	s.Sees("Choose where it is usually kept.")
	s.Sees("Can't be below 0.")
	s.Sees("One decimal place at most.")
	s.Sees("Required.")
	s.Sees("Type a number.")
	s.Sees("No Ingredients yet.")
	form := apptest.Form(t, s.Page().Body, "ingredient-add")
	if form.Get("name") != "egg" || form.Get("kcal") != "70" || form.Get("carbs") != "0.45" {
		t.Errorf("the refused add form holds name %q, kcal %q, carbs %q; want what was typed", form.Get("name"), form.Get("kcal"), form.Get("carbs"))
	}

	s.AddsIngredient(apptest.Ingredient{Unit: "g", Usual: "pantry", Kcal: "1", Protein: "1", Carbs: "1", Fat: "1"})
	s.IsRefused()
	s.Sees("Give it a name.")
}

// One food is one Ingredient: a name that normalises as an existing one's, with a size
// or grade word, is refused on the name, naming the one the User has. A plural, which no
// rule tells from another food, is added.
func TestAnIngredientOfAFoodTheUserHasIsRefused(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.AddsIngredient(egg)

	large := egg
	large.Name = "Large Egg"
	s.AddsIngredient(large)
	s.IsRefused()
	s.Sees("You already have egg: one food is one Ingredient.")
	if got := apptest.Form(t, s.Page().Body, "ingredient-add").Get("name"); got != "Large Egg" {
		t.Errorf("the refused add form holds name %q, want Large Egg", got)
	}

	white := egg
	white.Name = "egg white"
	s.AddsIngredient(white)
	s.Sees("Added egg white.")
}

// Italian names are normalised as Italian, and values shown with a decimal comma. Fish
// is not peach, though only a letter tells them apart.
func TestIngredientsInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{
		Name: "uovo", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6,3", Carbs: "0.4", Fat: "4.8",
	})
	s.Sees("Aggiunto: uovo.")
	s.Sees("uovo frigo per pezzo: 70 kcal proteine 6,3 g carboidrati 0,4 g grassi 4,8 g fibre – non noto zuccheri – non noto")

	s.AddsIngredient(apptest.Ingredient{
		Name: "Uovo grande", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6", Carbs: "0", Fat: "5",
	})
	s.IsRefused()
	s.Sees("Hai già uovo: un cibo è un solo Ingrediente.")

	peach := apptest.Ingredient{Unit: "g", Usual: "fridge", Kcal: "39", Protein: "0.9", Carbs: "9.5", Fat: "0.3"}
	peach.Name = "pesca"
	s.AddsIngredient(peach)
	fish := peach
	fish.Name, fish.Kcal = "pesce", "120"
	s.AddsIngredient(fish)
	s.Sees("Aggiunto: pesce.")
}

// Posted from the side sheet by htmx, the add form answers with the sheet's body alone:
// the add confirmed and the form cleared, or refused with what was typed.
func TestTheIngredientsSheetAddsInPlace(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	sheet := s.Get(t, "/ingredients/sheet")
	if sheet.StatusCode != http.StatusOK || strings.Contains(sheet.Body, "<html") {
		t.Fatalf("the sheet's body answered %d, want 200 and a fragment:\n%s", sheet.StatusCode, sheet.Body)
	}

	form := apptest.Form(t, sheet.Body, "ingredient-add")
	egg.Fill(form)
	added := s.HTMXPostForm(t, form.Action, form.Values)
	if added.StatusCode != http.StatusOK || strings.Contains(added.Body, "<html") {
		t.Fatalf("adding from the sheet answered %d, want 200 and a fragment:\n%s", added.StatusCode, added.Body)
	}
	if shown := apptest.Text(added.Body); !strings.Contains(shown, "Added egg.") || !strings.Contains(shown, "egg fridge per piece") {
		t.Errorf("the sheet after adding egg shows: %s", shown)
	}
	if got := apptest.Form(t, added.Body, "ingredient-add").Get("name"); got != "" {
		t.Errorf("the add form after adding holds name %q, want it cleared", got)
	}

	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("adding egg again from the sheet answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if !strings.Contains(apptest.Text(refused.Body), "You already have egg") {
		t.Errorf("the sheet refusing egg again shows: %s", apptest.Text(refused.Body))
	}
	if got := apptest.Form(t, refused.Body, "ingredient-add").Get("name"); got != "egg" {
		t.Errorf("the refused add form holds name %q, want egg", got)
	}
}

// One User never sees another's Ingredients, and may add the same food.
func TestEachUserHasTheirOwnIngredients(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Invite("bea@example.com")
	s.Open("/ingredients")
	s.AddsIngredient(egg)

	s.SwitchesTo("bea@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.Sees("No Ingredients yet.")
	s.AddsIngredient(egg)
	s.Sees("Added egg.")
}
