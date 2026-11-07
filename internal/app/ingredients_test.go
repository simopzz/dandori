package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

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
	s.IsConfirmed("Added egg.")
	s.Sees("egg fridge per piece: 70 kcal protein 6.3 g carbs 0.4 g fat 4.8 g fibre – unknown sugar – unknown")
	s.DoesNotSee("No Ingredients yet.")

	s.AddsIngredient(apptest.Ingredient{
		Name: "Rolled oats", Unit: "g", Usual: "pantry", Kcal: "372", Protein: "13.5", Carbs: "58.7", Fat: "7",
		Fibre: "10.1", Sugar: "0",
	})
	s.AddsIngredient(apptest.Ingredient{
		Name: "olive oil", Unit: "ml", Usual: "pantry", Kcal: "824", Protein: "0", Carbs: "0", Fat: "91.6", Staple: true,
	})
	s.IsConfirmed("Added olive oil.")
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
	s.IsConfirmed("Added egg white.")
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
	s.IsConfirmed("Aggiunto: uovo.")
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
	s.IsRefused()
	s.Sees("Intendevi pesca?")
	s.Submits("ingredient-add")
	s.IsConfirmed("Aggiunto: pesce.")
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
	if shown := apptest.Text(added.Body); !strings.Contains(shown, "egg fridge per piece") {
		t.Errorf("the sheet after adding egg shows: %s", shown)
	}
	if notices := apptest.Notices(t, added.Body); !slices.Equal(notices, []string{"Added egg."}) {
		t.Errorf("adding egg from the sheet confirms %q", notices)
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
	s.IsConfirmed("Added egg.")
}

// A name close to an Ingredient's, which doesn't normalise to it, suggests that one. The
// User picks it, adding nothing and finding it in the list, or confirms the name they
// typed and adds it. A confirmation is of the name shown: changed, it is matched again.
func TestANearMissSuggestsTheIngredientItMeant(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.AddsIngredient(egg)

	eggs := egg
	eggs.Name = "eggs"
	s.AddsIngredient(eggs)
	s.IsRefused()
	s.Sees("Did you mean egg?")
	if got := apptest.Form(t, s.Page().Body, "ingredient-add").Get("name"); got != "eggs" {
		t.Errorf("the add form suggesting egg holds name %q, want eggs", got)
	}

	s.Follows("Use egg")
	s.IsAt("/ingredients")
	s.IsConfirmed("egg is already one of your Ingredients.")
	s.DoesNotSee("eggs")
	if got := apptest.Form(t, s.Page().Body, "ingredient-add").Get("name"); got != "" {
		t.Errorf("the add form after picking egg holds name %q, want it cleared", got)
	}
	if got := apptest.Form(t, s.Page().Body, "ingredient-search").Get("q"); got != "egg" {
		t.Errorf("the search after picking egg holds %q, want egg", got)
	}

	s.AddsIngredient(eggs)
	s.Sees("Did you mean egg?")
	typo := eggs
	typo.Name = "eggz"
	s.AddsIngredient(typo)
	s.IsRefused()
	s.Sees("Did you mean egg?")
	s.Submits("ingredient-add")
	s.IsConfirmed("Added eggz.")
}

// The suggestion, picking it and confirming the name typed each answer with the side
// sheet's body alone when htmx posts them. Picking a name the User has no Ingredient of
// confirms nothing.
func TestTheIngredientsSheetSuggestsInPlace(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	sheet := s.Get(t, "/ingredients/sheet")
	form := apptest.Form(t, sheet.Body, "ingredient-add")
	egg.Fill(form)
	s.HTMXPostForm(t, form.Action, form.Values)

	form.Values.Set("name", "eggs")
	suggested := s.HTMXPostForm(t, form.Action, form.Values)
	if suggested.StatusCode != http.StatusUnprocessableEntity || strings.Contains(suggested.Body, "<html") {
		t.Fatalf("adding eggs from the sheet answered %d, want 422 and a fragment:\n%s", suggested.StatusCode, suggested.Body)
	}
	if shown := apptest.Text(suggested.Body); !strings.Contains(shown, "Did you mean egg?") {
		t.Errorf("the sheet adding eggs shows: %s", shown)
	}

	picked := s.HTMXGet(t, "/ingredients/sheet?picked=egg")
	if picked.StatusCode != http.StatusOK || strings.Contains(picked.Body, "<html") {
		t.Fatalf("picking egg from the sheet answered %d, want 200 and a fragment:\n%s", picked.StatusCode, picked.Body)
	}
	if shown := apptest.Text(picked.Body); strings.Contains(shown, "eggs") {
		t.Errorf("the sheet after picking egg shows: %s", shown)
	}
	if notices := apptest.Notices(t, picked.Body); !slices.Equal(notices, []string{"egg is already one of your Ingredients."}) {
		t.Errorf("picking egg from the sheet confirms %q", notices)
	}

	if notices := apptest.Notices(t, s.HTMXGet(t, "/ingredients/sheet?picked=salmone").Body); len(notices) > 0 {
		t.Errorf("picking an Ingredient the User doesn't have confirms %q", notices)
	}

	confirm := apptest.Form(t, suggested.Body, "ingredient-add")
	added := s.HTMXPostForm(t, confirm.Action, confirm.Values)
	if added.StatusCode != http.StatusOK || strings.Contains(added.Body, "<html") {
		t.Fatalf("confirming eggs from the sheet answered %d, want 200 and a fragment:\n%s", added.StatusCode, added.Body)
	}
	if notices := apptest.Notices(t, added.Body); !slices.Equal(notices, []string{"Added eggs."}) {
		t.Errorf("confirming eggs from the sheet confirms %q", notices)
	}
}

// Leaving the name asks for a suggestion before the form is sent: the one it may have
// meant, carrying the name's confirmation, or nothing for another food, a blank name, or
// a name the User has, which adding refuses.
func TestANameIsSuggestedForOnLeavingIt(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.AddsIngredient(egg)

	near := s.HTMXGet(t, "/ingredients/suggestion?name=eggs")
	if near.StatusCode != http.StatusOK || strings.Contains(near.Body, "<html") {
		t.Fatalf("a suggestion for eggs answered %d, want 200 and a fragment:\n%s", near.StatusCode, near.Body)
	}
	if shown := apptest.Text(near.Body); !strings.Contains(shown, "Did you mean egg?") {
		t.Errorf("the suggestion for eggs shows: %s", shown)
	}
	if got := apptest.Link(t, near.Body, "Use egg"); got != "/ingredients?picked=egg" {
		t.Errorf("Use egg leads to %q", got)
	}
	if !strings.Contains(near.Body, `name="confirm" value="eggs"`) {
		t.Errorf("the suggestion for eggs doesn't confirm eggs:\n%s", near.Body)
	}

	for _, name := range []string{"salmon", "", "Large%20egg"} {
		if got := s.HTMXGet(t, "/ingredients/suggestion?name="+name); got.StatusCode != http.StatusOK || strings.TrimSpace(got.Body) != "" {
			t.Errorf("the suggestion for %q answered %d: %s", name, got.StatusCode, got.Body)
		}
	}
}

// A search ranks the Ingredients by spelling, the most alike first, so a typo still finds
// the Ingredient; one that finds nothing says so, and a blank one lists them all.
func TestSearchingIngredientsBySpelling(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	for _, name := range []string{"polenta", "petto di pollo", "uovo"} {
		s.AddsIngredient(apptest.Ingredient{Name: name, Unit: "g", Usual: "fridge", Kcal: "100", Protein: "1", Carbs: "1", Fat: "1"})
	}

	s.SearchesIngredients("polo")
	s.IsAt("/ingredients")
	s.DoesNotSee("uovo")
	shown := apptest.Text(s.Page().Body)
	pollo, polenta := strings.Index(shown, "petto di pollo frigo"), strings.Index(shown, "polenta frigo")
	if pollo < 0 || polenta < 0 || polenta < pollo {
		t.Errorf("searching for polo shows, want petto di pollo then polenta: %s", shown)
	}
	if got := apptest.Form(t, s.Page().Body, "ingredient-search").Get("q"); got != "polo" {
		t.Errorf("the search holds %q, want polo", got)
	}

	s.SearchesIngredients("salmone")
	s.Sees("Nessun Ingrediente somiglia a salmone.")

	s.SearchesIngredients("")
	s.Sees("uovo frigo")
	s.Sees("polenta frigo")

	list := s.HTMXGet(t, "/ingredients/list?q=polo")
	if list.StatusCode != http.StatusOK || strings.Contains(list.Body, "<html") || strings.Contains(list.Body, "ingredient-add") {
		t.Fatalf("searching with htmx answered %d, want 200 and the list alone:\n%s", list.StatusCode, list.Body)
	}
	if shown := apptest.Text(list.Body); !strings.Contains(shown, "petto di pollo") || strings.Contains(shown, "uovo") {
		t.Errorf("searching for polo with htmx shows: %s", shown)
	}
}

// An Ingredient is edited in place: its name, its nutritional values, fibre and sugar
// becoming unknown, and its usual Storage Location, at any time.
func TestEditingAnIngredient(t *testing.T) {
	s := kitchenWith(t, apptest.Ingredient{
		Name: "Rolled oats", Unit: "g", Usual: "pantry", Kcal: "372", Protein: "13.5", Carbs: "58.7", Fat: "7",
		Fibre: "10.1", Sugar: "0",
	})
	if got := apptest.EditForm(t, s.Page().Body, "Rolled oats"); got.Get("kcal") != "372" || got.Get("fibre") != "10.1" || got.Get("unit") != "g" {
		t.Errorf("the edit form of Rolled oats holds kcal %q, fibre %q, unit %q; want its values", got.Get("kcal"), got.Get("fibre"), got.Get("unit"))
	}

	s.EditsIngredient("Rolled oats", apptest.Ingredient{
		Name: "oat flakes", Unit: "g", Usual: "fridge", Kcal: "370", Protein: "13", Carbs: "59", Fat: "7",
	})
	s.IsAt("/ingredients")
	s.IsConfirmed("Saved oat flakes.")
	s.Sees("oat flakes fridge per 100 g: 370 kcal protein 13 g carbs 59 g fat 7 g fibre – unknown sugar – unknown")
	s.DoesNotSee("Rolled oats")

	s.EditsIngredient("oat flakes", apptest.Ingredient{Name: "Oat Flakes", Unit: "g", Usual: "fridge", Kcal: "", Protein: "13", Carbs: "59", Fat: "7"})
	s.IsRefused()
	s.Sees("Required.")
	if got := apptest.EditForm(t, s.Page().Body, "Oat Flakes"); got.Get("protein") != "13" {
		t.Errorf("the refused edit form holds protein %q, want what was typed", got.Get("protein"))
	}
	s.Open("/ingredients")
	s.Sees("oat flakes fridge per 100 g: 370 kcal")
}

// A rename that normalises to another Ingredient's name is refused on the name, naming
// that one; renaming to the Ingredient's own name in other words is not. A new name only
// close to another's suggests it, and saving it again confirms the name typed.
func TestARenameToAnotherIngredientIsRefused(t *testing.T) {
	s := kitchenWith(t, egg, rice)

	riceAsEgg := rice
	riceAsEgg.Name = "Large egg"
	s.EditsIngredient("rice", riceAsEgg)
	s.IsRefused()
	s.Sees("You already have egg: one food is one Ingredient.")
	if got := apptest.EditForm(t, s.Page().Body, "Large egg").Get("kcal"); got != "350" {
		t.Errorf("the refused edit form holds kcal %q, want what was typed", got)
	}
	s.Open("/ingredients")
	s.Sees("rice pantry")

	bigEgg := egg
	bigEgg.Name = "Egg"
	s.EditsIngredient("egg", bigEgg)
	s.IsConfirmed("Saved Egg.")

	riceAsEggs := rice
	riceAsEggs.Name = "eggs"
	s.EditsIngredient("rice", riceAsEggs)
	s.IsRefused()
	s.Sees("Did you mean Egg?")
	s.Submits(editFormID(t, s.Page().Body, "eggs"))
	s.IsConfirmed("Saved eggs.")
	s.DoesNotSee("rice")
}

// editFormID is the id of the edit form on page holding the name name.
func editFormID(t *testing.T, page, name string) string {
	t.Helper()
	action := apptest.EditForm(t, page, name).Action
	return "ingredient-edit-" + strings.TrimPrefix(action, "/ingredients/")
}

// The unit changes while no Movement refers to the Ingredient. Once one does, it is
// refused on the unit, saying the Movements are recorded in it, even with the Stock Item
// counted to nothing; the rest of the edit is not saved either.
func TestTheUnitIsFixedOnceStockIsCounted(t *testing.T) {
	s := kitchenWith(t, rice)
	inPieces := rice
	inPieces.Unit = "piece"
	s.EditsIngredient("rice", inPieces)
	s.IsConfirmed("Saved rice.")
	s.Sees("rice pantry per piece")
	s.EditsIngredient("rice", rice)
	s.Sees("rice pantry per 100 g")

	s.Counts("rice", "", "700", "")
	s.Counts("rice", "", "0", "")
	s.Open("/ingredients")
	inPieces.Kcal = "360"
	s.EditsIngredient("rice", inPieces)
	s.IsRefused()
	s.Sees("Its Movements are recorded in g, so its unit stays, even counted to nothing.")
	if got := apptest.EditForm(t, s.Page().Body, "rice").Get("unit"); got != "piece" {
		t.Errorf("the refused edit form holds unit %q, want what was chosen", got)
	}
	s.Open("/ingredients")
	s.Sees("rice pantry per 100 g: 350 kcal")
}

// Editing nutrition, or renaming, changes no Movement: the Kitchen page holds what it
// held, under the new name.
func TestEditingAnIngredientLeavesTheKitchenAsItWas(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "700", "")
	s.Open("/ingredients")
	brown := rice
	brown.Name, brown.Kcal, brown.Fibre, brown.Usual = "brown rice", "362", "3.4", "fridge"
	s.EditsIngredient("rice", brown)
	s.IsConfirmed("Saved brown rice.")

	s.Open("/kitchen")
	s.Sees("01 Pantry brown rice 700 g")
	s.DoesNotSee("Fridge")
}

// An Ingredient nothing refers to is deleted. One with a Movement is refused, saying why,
// and the database refuses it as well.
func TestDeletingAnIngredient(t *testing.T) {
	s := kitchenWith(t, egg, rice)
	s.DeletesIngredient("egg")
	s.IsAt("/ingredients")
	s.IsConfirmed("Deleted egg.")
	s.DoesNotSee("egg")
	s.Sees("rice pantry")

	s.Counts("rice", "", "0", "")
	s.Open("/ingredients")
	s.DeletesIngredient("rice")
	s.IsRefused()
	s.Sees("It has Movements, so it can't be deleted: the Kitchen Ledger keeps every one.")
	s.Open("/ingredients")
	s.Sees("rice pantry")

	s.Refuses(t, `DELETE FROM ingredient WHERE name = 'rice'`)
}

// Posted from the side sheet by htmx, editing and deleting answer with the sheet's body
// alone, searched as it was: confirmed with the row closed, or refused with the row open
// as typed and why.
func TestTheIngredientsSheetEditsAndDeletesInPlace(t *testing.T) {
	s := kitchenWith(t, egg, rice)
	s.Counts("rice", "", "700", "")
	sheet := s.HTMXGet(t, "/ingredients/list?q=rice")
	fragment := func(what string, got apptest.Response, status int) string {
		t.Helper()
		if got.StatusCode != status || strings.Contains(got.Body, "<html") {
			t.Fatalf("%s from the sheet answered %d, want %d and a fragment:\n%s", what, got.StatusCode, status, got.Body)
		}
		return apptest.Text(got.Body)
	}

	edit := apptest.EditForm(t, sheet.Body, "rice")
	edit.Values.Set("q", "rice")
	edit.Values.Set("kcal", "360")
	saved := s.HTMXPostForm(t, edit.Action, edit.Values)
	if shown := fragment("saving rice", saved, http.StatusOK); !strings.Contains(shown, "rice pantry per 100 g: 360 kcal") || strings.Contains(shown, "egg") {
		t.Errorf("the sheet after saving rice, searched for rice, shows: %s", shown)
	}
	if notices := apptest.Notices(t, saved.Body); !slices.Equal(notices, []string{"Saved rice."}) {
		t.Errorf("saving rice from the sheet confirms %q", notices)
	}
	if got := apptest.Form(t, saved.Body, "ingredient-search").Get("q"); got != "rice" {
		t.Errorf("the search after saving rice holds %q, want rice", got)
	}
	if strings.Contains(saved.Body, "<details open") {
		t.Errorf("a row is open after saving rice:\n%s", saved.Body)
	}

	edit.Values.Set("unit", "ml")
	fixed := s.HTMXPostForm(t, edit.Action, edit.Values)
	if shown := fragment("changing rice's unit", fixed, http.StatusUnprocessableEntity); !strings.Contains(shown, "Its Movements are recorded in g") {
		t.Errorf("the sheet refusing rice's unit shows: %s", shown)
	}
	if !strings.Contains(fixed.Body, "<details open") || apptest.EditForm(t, fixed.Body, "rice").Get("unit") != "ml" {
		t.Errorf("the refused row isn't open as typed:\n%s", fixed.Body)
	}

	del := apptest.DeleteForm(t, saved.Body, "rice")
	refused := s.HTMXPostForm(t, del.Action, del.Values)
	if shown := fragment("deleting rice", refused, http.StatusUnprocessableEntity); !strings.Contains(shown, "It has Movements, so it can't be deleted") {
		t.Errorf("the sheet refusing to delete rice shows: %s", shown)
	}
	if !strings.Contains(refused.Body, "<details open") {
		t.Errorf("the row refused deleting isn't open:\n%s", refused.Body)
	}

	del = apptest.DeleteForm(t, s.HTMXGet(t, "/ingredients/list").Body, "egg")
	deleted := s.HTMXPostForm(t, del.Action, del.Values)
	if shown := fragment("deleting egg", deleted, http.StatusOK); strings.Contains(shown, "egg fridge") {
		t.Errorf("the sheet after deleting egg shows: %s", shown)
	}
	if notices := apptest.Notices(t, deleted.Body); !slices.Equal(notices, []string{"Deleted egg."}) {
		t.Errorf("deleting egg from the sheet confirms %q", notices)
	}
}

// A User can neither edit nor delete another's Ingredient.
func TestAnotherUsersIngredientCantBeEditedOrDeleted(t *testing.T) {
	s := kitchenWith(t, rice)
	edit := apptest.EditForm(t, s.Page().Body, "rice")
	del := apptest.DeleteForm(t, s.Page().Body, "rice")

	s.Invite("bea@example.com")
	s.SwitchesTo("bea@example.com")
	s.ChoosesLanguage("English")
	if got := s.PostForm(t, edit.Action, edit.Values); got.StatusCode != http.StatusNotFound {
		t.Errorf("editing another User's Ingredient answered %d, want 404", got.StatusCode)
	}
	if got := s.PostForm(t, del.Action, del.Values); got.StatusCode != http.StatusNotFound {
		t.Errorf("deleting another User's Ingredient answered %d, want 404", got.StatusCode)
	}
	s.SwitchesTo("ada@example.com")
	s.Open("/ingredients")
	s.Sees("rice pantry")
}

// In the side sheet, an Ingredient's row opens to edit it, and saving closes it in place,
// the sheet staying open; a refused delete keeps it open with why, and a delete takes
// the Ingredient off the list.
func TestBrowserEditsAndDeletesInTheSideSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredients"}).First().Click(); err != nil {
		t.Fatalf("open the Ingredients: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	fillsIngredient(t, sheet, "rice")
	b.IsConfirmed(t, "Added rice.")

	expect := playwright.NewPlaywrightAssertions()
	row := sheet.Locator("li.row").Filter(playwright.LocatorFilterOptions{HasText: "rice"})
	if err := row.Locator("summary").Click(); err != nil {
		t.Fatalf("open rice's row: %v", err)
	}
	name := row.GetByLabel("Name", playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(true)})
	if err := name.Fill("brown rice"); err != nil {
		t.Fatalf("rename rice: %v", err)
	}
	if err := row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save"}).Click(); err != nil {
		t.Fatalf("save rice: %v", err)
	}
	b.IsConfirmed(t, "Saved brown rice.")
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Errorf("the sheet closes on saving: %v", err)
	}
	b.Shot(t, "ingredient-saved")
	if err := expect.Locator(sheet.Locator("details[open]")).ToHaveCount(0); err != nil {
		t.Errorf("a row stays open after saving: %v", err)
	}

	if err := row.Locator("summary").Click(); err != nil {
		t.Fatalf("open brown rice's row: %v", err)
	}
	b.Shot(t, "ingredient-edit")
	if err := row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Delete"}).Click(); err != nil {
		t.Fatalf("delete brown rice: %v", err)
	}
	b.IsConfirmed(t, "Deleted brown rice.")
	if err := expect.Locator(sheet).ToContainText("No Ingredients yet."); err != nil {
		t.Errorf("the sheet still lists brown rice: %v", err)
	}
}
