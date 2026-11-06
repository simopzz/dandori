package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// rice and salt are a food measured in grams and a Staple, beside egg, in pieces.
var (
	rice = apptest.Ingredient{Name: "rice", Unit: "g", Usual: "pantry", Kcal: "350", Protein: "7", Carbs: "78", Fat: "0.6"}
	salt = apptest.Ingredient{Name: "salt", Unit: "g", Usual: "pantry", Kcal: "0", Protein: "0", Carbs: "0", Fat: "0", Staple: true}
)

// kitchenWith signs a new User in, in English, with these Ingredients.
func kitchenWith(t *testing.T, ings ...apptest.Ingredient) *apptest.Scenario {
	t.Helper()
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	for _, ing := range ings {
		s.AddsIngredient(ing)
	}
	return s
}

// A Count says how much a Storage Location holds, by default the Ingredient's usual one,
// and the Kitchen page shows what the Kitchen Ledger adds up to, grouped by Storage
// Location, in the unit of each.
func TestCountingWhatTheKitchenHolds(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.Open("/kitchen")
	s.Sees("Nothing in your kitchen yet. Count what you have, or add an Ingredient to start.")

	s.Counts("rice", "", "700", "")
	s.IsAt("/kitchen/count")
	s.Sees("Counted rice: 700 g in the pantry.")
	if got := apptest.Form(t, s.Page().Body, "count-search").Get("q"); got != "" {
		t.Errorf("the search after a Count holds %q, want it cleared", got)
	}

	s.Counts("egg", "", "1.5", "")
	s.Sees("Counted egg: 1.5 pieces in the fridge.")
	s.Open("/kitchen")
	s.DoesNotSee("Nothing in your kitchen yet.")
	s.Sees("01 Pantry rice 700 g")
	s.Sees("02 Fridge egg 1.5 pieces")
	s.DoesNotSee("Freezer rice")
}

// The form shows what Dandori believes each Storage Location holds. A Storage Location
// other than the usual one is a Stock Item of its own, and a later Count corrects what
// the one before it left.
func TestACountShowsWhatDandoriBelieves(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "200", "")

	s.Open("/kitchen/count")
	s.Follows("rice")
	s.Sees("pantry 200 g")
	s.Sees("fridge 0 g")
	s.Sees("freezer 0 g")
	if got := apptest.Form(t, s.Page().Body, "count-form").Get("location"); got != "pantry" {
		t.Errorf("the Count form holds Storage Location %q, want the usual pantry", got)
	}

	s.Counts("rice", "freezer", "300", "for later")
	s.Sees("Counted rice: 300 g in the freezer.")
	s.Counts("rice", "", "700", "")
	s.Open("/kitchen")
	s.Sees("Pantry rice 700 g")
	s.Sees("Freezer rice 300 g")
}

// A Stock Item's Count is prefilled with its Ingredient and Storage Location, leaving the
// quantity to state.
func TestAStockItemHasItsOwnCount(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "freezer", "300", "")
	s.Open("/kitchen")

	s.Follows("Count rice")
	s.IsAt("/kitchen/count")
	form := apptest.Form(t, s.Page().Body, "count-form")
	if form.Get("location") != "freezer" || form.Get("stated") != "" {
		t.Errorf("the Stock Item's Count holds Storage Location %q and quantity %q, want freezer and none", form.Get("location"), form.Get("stated"))
	}
	s.Sees("freezer 300 g")
}

// A Count to nothing takes the Stock Item off the Kitchen page, and one that changes
// nothing is still a Count, at its own time.
func TestCountingToNothing(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.Counts("rice", "", "700", "")
	s.Counts("egg", "", "6", "")

	s.Counts("rice", "", "0", "it spoiled")
	s.Sees("Counted rice: 0 g in the pantry.")
	s.Open("/kitchen")
	s.DoesNotSee("rice 700 g")
	s.DoesNotSee("Count rice")
	s.DoesNotSee("Pantry")
	s.Sees("01 Fridge egg 6 pieces")
}

// A Count happens at the moment it is saved, which the form has no say in: each Stock
// Item shows when it was last counted, by the time alone today and with its day before.
func TestEveryCountHappensWhenItIsSaved(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.At("Mon 14:05")
	s.Counts("rice", "", "700", "")
	s.At("Tue 09:30")
	s.Counts("egg", "", "6", "")

	s.Open("/kitchen")
	s.Sees("rice 700 g counted on Monday 2 November, 14:05")
	s.Sees("egg 6 pieces counted at 09:30")

	s.At("Tue 10:45")
	s.Counts("rice", "", "700", "the same as it was")
	s.Open("/kitchen")
	s.Sees("rice 700 g counted at 10:45")
	s.Sees("egg 6 pieces counted at 09:30")
}

// A Count below 0 or finer than its unit is refused on the field, keeping what was typed
// and leaving the kitchen as it was.
func TestARefusedCount(t *testing.T) {
	cases := []struct {
		name, ingredient, stated, why string
	}{
		{"below nothing", "rice", "-1", "Can't be below 0."},
		{"part of a gram", "rice", "12.5", "Grams and millilitres are whole numbers."},
		{"two decimal places of a piece", "egg", "0.25", "One decimal place at most."},
		{"not a number", "rice", "lots", "Type a number."},
		{"nothing stated", "rice", "", "Say how much is there."},
		{"too much", "rice", "1000000", "Must be below 1000000."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := kitchenWith(t, rice, egg)
			s.Counts(c.ingredient, "", c.stated, "a note")
			s.IsRefused()
			s.Sees(c.why)
			form := apptest.Form(t, s.Page().Body, "count-form")
			if form.Get("stated") != c.stated || form.Get("note") != "a note" {
				t.Errorf("the refused Count holds %q and note %q, want what was typed", form.Get("stated"), form.Get("note"))
			}
			s.Open("/kitchen")
			s.Sees("Nothing in your kitchen yet.")
		})
	}
}

// A Storage Location that isn't one is refused on the field.
func TestACountNeedsAStorageLocation(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Open("/kitchen/count")
	s.Follows("rice")
	form := apptest.Form(t, s.Page().Body, "count-form")
	form.Values.Set("location", "cellar")
	form.Values.Set("stated", "700")

	refused := s.PostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apptest.Text(refused.Body), "Choose where it is kept.") {
		t.Errorf("counting in a cellar answered %d: %s", refused.StatusCode, apptest.Text(refused.Body))
	}
}

// Staples are never counted: the search doesn't offer them, the Count form doesn't open
// on one, and a post for one is refused.
func TestStaplesAreNeverOffered(t *testing.T) {
	s := kitchenWith(t, rice, salt)
	s.Open("/kitchen/count")
	s.Sees("rice")
	s.DoesNotSee("salt")

	s.Open("/kitchen/count?ingredient=" + ingredientID(t, s, "salt"))
	s.DoesNotSee("salt")
	apptest.Form(t, s.Page().Body, "count-search")

	refused := s.PostForm(t, "/kitchen/count", url.Values{
		"ingredient": {ingredientID(t, s, "salt")}, "location": {"pantry"}, "stated": {"100"},
	})
	if refused.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apptest.Text(refused.Body), "salt is a Staple: it is never counted.") {
		t.Errorf("counting a Staple answered %d: %s", refused.StatusCode, apptest.Text(refused.Body))
	}
	s.Open("/kitchen")
	s.Sees("Nothing in your kitchen yet.")
}

// A search for a Staple says why it isn't offered, and doesn't offer to add it again,
// which would be refused. A search that only resembles one still offers the new name.
func TestSearchingForAStapleSaysWhy(t *testing.T) {
	s := kitchenWith(t, rice, salt)
	s.Open("/kitchen/count?q=Salt")
	s.Sees("salt is a Staple: it is never counted.")
	s.DoesNotSee("as a new Ingredient")

	s.Open("/kitchen/count?q=salty")
	s.Sees("Add salty as a new Ingredient")
}

// A confirmation tells the Count it confirms, as it was saved, whatever came after it,
// and an id that is no Count confirms nothing.
func TestAConfirmationTellsTheCountSaved(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "700", "")
	var first string
	s.Scan(t, &first, `SELECT max(id)::text FROM movement`)
	s.Counts("rice", "", "300", "")

	s.Open("/kitchen/count?counted=" + first)
	s.Sees("Counted rice: 700 g in the pantry.")
	s.Open("/kitchen/count?counted=999999")
	s.DoesNotSee("Counted")
}

// ingredientID is the id of the Ingredient name, for a post no page offers.
func ingredientID(t *testing.T, s *apptest.Scenario, name string) string {
	t.Helper()
	var id string
	s.Scan(t, &id, `SELECT id::text FROM ingredient WHERE name = $1`, name)
	return id
}

// A search that finds nothing offers to add what was typed, starting the add form on it.
func TestACountSearchFindingNothingOffersANewIngredient(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Open("/kitchen/count?q=salmon")
	s.Sees("No Ingredient is like salmon.")
	s.Follows("Add salmon as a new Ingredient")
	s.IsAt("/ingredients")
	if got := apptest.Form(t, s.Page().Body, "ingredient-add").Get("name"); got != "salmon" {
		t.Errorf("the add form holds name %q, want salmon", got)
	}

	sheet := s.HTMXGet(t, "/ingredients/sheet?name=salmon")
	if got := apptest.Form(t, sheet.Body, "ingredient-add").Get("name"); got != "salmon" {
		t.Errorf("the add form in the sheet holds name %q, want salmon", got)
	}
}

// An Ingredient added from a Count's offer is there to count: the add lands on its Count
// form. A refused add keeps that intention, and a Staple, which is never counted, stays
// an ordinary add. Added any other way, it stays on the Ingredients.
func TestAnIngredientAddedToCountOpensItsCount(t *testing.T) {
	s := kitchenWith(t)
	s.Open("/kitchen/count?q=salmon")
	s.Follows("Add salmon as a new Ingredient")

	refused := apptest.Ingredient{Name: "salmon", Unit: "g", Usual: "fridge", Kcal: "-1", Protein: "20", Carbs: "0", Fat: "13"}
	s.AddsIngredient(refused)
	s.IsRefused()
	salmon := refused
	salmon.Kcal = "208"
	s.AddsIngredient(salmon)
	s.IsAt("/kitchen/count")
	form := apptest.Form(t, s.Page().Body, "count-form")
	if got := form.Get("location"); got != "fridge" {
		t.Errorf("the new Ingredient's Count holds Storage Location %q, want its usual fridge", got)
	}
	s.Sees("salmon")

	pepper := salt
	pepper.Name = "pepper"
	s.Open("/kitchen/count?q=pepper")
	s.Follows("Add pepper as a new Ingredient")
	s.AddsIngredient(pepper)
	s.IsAt("/ingredients")
	s.Sees("Added pepper.")

	s.Open("/ingredients")
	s.AddsIngredient(rice)
	s.IsAt("/ingredients")
	s.Sees("Added rice.")
}

// Posted from the sheet by htmx, an add to count answers with the Count sheet's body,
// retargeted there, leaving the Ingredients sheet's own swap for every other add.
func TestAnIngredientAddedToCountSwapsTheCountSheet(t *testing.T) {
	s := kitchenWith(t)
	form := apptest.Form(t, s.HTMXGet(t, "/ingredients/sheet?name=salmon&counting=1").Body, "ingredient-add")
	salmon := apptest.Ingredient{Name: "salmon", Unit: "g", Usual: "fridge", Kcal: "208", Protein: "20", Carbs: "0", Fat: "13"}
	salmon.Fill(form)

	added := s.HTMXPostForm(t, form.Action, form.Values)
	if added.StatusCode != http.StatusOK || strings.Contains(added.Body, "<html") {
		t.Fatalf("adding to count answered %d, want 200 and a fragment:\n%s", added.StatusCode, added.Body)
	}
	if got := added.Header.Get("HX-Retarget"); got != "#count-body" {
		t.Errorf("adding to count retargets %q, want the Count sheet's body", got)
	}
	apptest.Form(t, added.Body, "count-form")

	plain := apptest.Form(t, s.HTMXGet(t, "/ingredients/sheet").Body, "ingredient-add")
	rice.Fill(plain)
	if got := s.HTMXPostForm(t, plain.Action, plain.Values).Header.Get("HX-Retarget"); got != "" {
		t.Errorf("an ordinary add retargets %q, want no retarget", got)
	}
}

// With no Ingredient to count, the Count form says how to start.
func TestCountingWithNoIngredients(t *testing.T) {
	s := kitchenWith(t)
	s.Open("/kitchen/count")
	s.Sees("No Ingredients to count yet.")
	s.Follows("Add an Ingredient")
	s.IsAt("/ingredients")
}

// As htmx posts them, the Count sheet's pieces answer with fragments: saved, its body
// cleared with the Count confirmed; refused, the form as typed; searched, the list alone.
func TestTheCountSheetWorksInPlace(t *testing.T) {
	s := kitchenWith(t, rice, salt)
	found := s.HTMXGet(t, "/kitchen/count/list?q=ric")
	if shown := apptest.Text(found.Body); found.StatusCode != http.StatusOK || strings.Contains(found.Body, "<html") || strings.Contains(found.Body, "count-search") || !strings.Contains(shown, "rice") {
		t.Fatalf("searching with htmx answered %d, want the list alone:\n%s", found.StatusCode, found.Body)
	}
	if shown := apptest.Text(s.HTMXGet(t, "/kitchen/count/list?q=salt").Body); strings.Contains(shown, "salt pantry") {
		t.Errorf("searching with htmx offers a Staple: %s", shown)
	}

	sheet := s.HTMXGet(t, "/kitchen/count/sheet?ingredient="+ingredientID(t, s, "rice"))
	if sheet.StatusCode != http.StatusOK || strings.Contains(sheet.Body, "<html") {
		t.Fatalf("the sheet's body answered %d, want 200 and a fragment:\n%s", sheet.StatusCode, sheet.Body)
	}
	form := apptest.Form(t, sheet.Body, "count-form")

	form.Values.Set("stated", "-1")
	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("a refused Count answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if got := apptest.Form(t, refused.Body, "count-form").Get("stated"); got != "-1" {
		t.Errorf("the refused Count holds %q, want -1", got)
	}

	form.Values.Set("stated", "700")
	saved := s.HTMXPostForm(t, form.Action, form.Values)
	if saved.StatusCode != http.StatusOK || strings.Contains(saved.Body, "<html") {
		t.Fatalf("saving a Count answered %d, want 200 and a fragment:\n%s", saved.StatusCode, saved.Body)
	}
	if shown := apptest.Text(saved.Body); !strings.Contains(shown, "Counted rice: 700 g in the pantry.") {
		t.Errorf("the sheet after saving shows: %s", shown)
	}
	if got := apptest.Form(t, saved.Body, "count-search").Get("q"); got != "" {
		t.Errorf("the search after saving holds %q, want it cleared", got)
	}

	stock := s.HTMXGet(t, "/kitchen/stock")
	if shown := apptest.Text(stock.Body); stock.StatusCode != http.StatusOK || strings.Contains(stock.Body, "<html") || !strings.Contains(shown, "rice 700 g") {
		t.Errorf("the stock with htmx answered %d: %s", stock.StatusCode, stock.Body)
	}
}

// Quantities are shown as the User's Language writes them.
func TestCountingInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "uovo", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6", Carbs: "0", Fat: "5"})
	s.AddsIngredient(apptest.Ingredient{Name: "cipolla", Unit: "piece", Usual: "pantry", Kcal: "40", Protein: "1", Carbs: "9", Fat: "0"})

	s.Counts("uovo", "", "1", "")
	s.Sees("Contato uovo: 1 pezzo in frigo.")
	s.Counts("cipolla", "", "0,5", "")
	s.Sees("Contato cipolla: 0,5 pezzi in dispensa.")
	s.Open("/kitchen")
	s.Sees("Dispensa cipolla 0,5 pezzi")
	s.Sees("Frigo uovo 1 pezzo")
}

// One User never sees another's Stock Items, nor counts their Ingredient.
func TestEachUserHasTheirOwnStock(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Invite("bea@example.com")
	s.Counts("rice", "", "700", "")
	id := ingredientID(t, s, "rice")

	s.SwitchesTo("bea@example.com")
	s.ChoosesLanguage("English")
	s.Open("/kitchen")
	s.Sees("Nothing in your kitchen yet.")
	s.DoesNotSee("rice")

	hers := s.PostForm(t, "/kitchen/count", url.Values{"ingredient": {id}, "location": {"pantry"}, "stated": {"1"}})
	if hers.StatusCode != http.StatusNotFound {
		t.Errorf("counting another User's Ingredient answered %d, want 404", hers.StatusCode)
	}
	s.Open("/kitchen/count?ingredient=" + id)
	s.DoesNotSee("rice")
}

// The Kitchen Ledger is append-only: the User's role reads and adds Movements and can do
// nothing else to them.
func TestMovementsAreAppendOnly(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "700", "")
	var reads, adds, changes bool
	s.Scan(t, &reads, `SELECT has_table_privilege('authenticated', 'movement', 'SELECT')`)
	s.Scan(t, &adds, `SELECT has_table_privilege('authenticated', 'movement', 'INSERT')`)
	s.Scan(t, &changes, `SELECT has_table_privilege('authenticated', 'movement', 'UPDATE, DELETE, TRUNCATE')`)
	if !reads || !adds || changes {
		t.Errorf("the User's role reads Movements: %v, adds them: %v, changes them: %v", reads, adds, changes)
	}
}

// A User deleted takes their whole kitchen with them, Kitchen Ledger included.
func TestDeletingAUserDeletesTheirKitchen(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "700", "")
	s.Exec(t, `DELETE FROM "user" WHERE email = 'ada@example.com'`)
}

// addsRiceInTheBrowser adds rice from the Ingredients sheet on the Kitchen page, and
// closes it.
func addsRiceInTheBrowser(t *testing.T, b *apptest.Browser) {
	t.Helper()
	if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredients"}).First().Click(); err != nil {
		t.Fatalf("open the Ingredients: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	fillsIngredient(t, sheet, "rice")
	if err := playwright.NewPlaywrightAssertions().Locator(sheet).ToContainText("Added rice"); err != nil {
		t.Fatalf("rice isn't added: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click(); err != nil {
		t.Fatalf("close the Ingredients: %v", err)
	}
}

// fillsIngredient fills the add form in sheet for a food in grams kept in the pantry,
// named name, and adds it.
func fillsIngredient(t *testing.T, sheet playwright.Locator, name string) {
	t.Helper()
	field := func(label string) playwright.Locator {
		return sheet.GetByLabel(label, playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(true)})
	}
	if err := field("Name").Fill(name); err != nil {
		t.Fatalf("fill the name: %v", err)
	}
	if _, err := sheet.GetByLabel("Unit").SelectOption(playwright.SelectOptionValues{Labels: &[]string{"g"}}); err != nil {
		t.Fatalf("choose the unit: %v", err)
	}
	if _, err := sheet.GetByLabel("Usually kept in").SelectOption(playwright.SelectOptionValues{Labels: &[]string{"pantry"}}); err != nil {
		t.Fatalf("choose where it is kept: %v", err)
	}
	for _, label := range []string{"Calories", "Protein", "Carbs", "Fat"} {
		if err := field(label).Fill("1"); err != nil {
			t.Fatalf("fill %s: %v", label, err)
		}
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add"}).Click(); err != nil {
		t.Fatalf("add the Ingredient: %v", err)
	}
}

// The Count sheet stays open after a save, cleared with the Count confirmed and focus
// back on the search, while the page under it lists the new Stock Item. It closes only
// when the User closes it.
func TestBrowserCountSheetStaysOpenUntilClosed(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	addsRiceInTheBrowser(t, b)
	expect := playwright.NewPlaywrightAssertions()

	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Fatalf("the Count sheet doesn't open: %v", err)
	}
	if err := sheet.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "rice"}).Click(); err != nil {
		t.Fatalf("choose rice: %v", err)
	}
	stated := sheet.GetByLabel("How much is there")
	if err := expect.Locator(stated).ToBeFocused(); err != nil {
		t.Errorf("choosing rice doesn't focus the quantity: %v", err)
	}
	if err := stated.Fill("700"); err != nil {
		t.Fatalf("state the quantity: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("save the Count: %v", err)
	}

	if err := expect.Locator(sheet).ToContainText("Counted rice: 700 g in the pantry."); err != nil {
		t.Fatalf("the saved Count isn't confirmed: %v", err)
	}
	search := sheet.GetByRole("searchbox")
	if err := expect.Locator(search).ToHaveValue(""); err != nil {
		t.Errorf("the sheet isn't cleared: %v", err)
	}
	if err := expect.Locator(search).ToBeFocused(); err != nil {
		t.Errorf("a save doesn't focus the search: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("rice"); err != nil {
		t.Errorf("the page under the sheet doesn't list what was counted: %v", err)
	}
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Errorf("the Count sheet closed on its own: %v", err)
	}
	b.IsAt(t, "/kitchen")

	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click(); err != nil {
		t.Fatalf("close the Count sheet: %v", err)
	}
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("the Count sheet doesn't close on request: %v", err)
	}

	// A Stock Item's own Count opens the same sheet on it, ready for the quantity.
	if err := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("open the Stock Item's Count: %v", err)
	}
	if err := expect.Locator(stated).ToBeFocused(); err != nil {
		t.Errorf("a Stock Item's Count doesn't focus the quantity: %v", err)
	}

	// Closing the sheet resets it: the + opens it empty, on the search, and stays open.
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click(); err != nil {
		t.Fatalf("close the Stock Item's Count: %v", err)
	}
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Fatalf("the reset opened the Count sheet again: %v", err)
	}
	if err := b.Locator("#count-search").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateAttached}); err != nil {
		t.Fatalf("closing the Count sheet doesn't reset it: %v", err)
	}
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("the reset opened the Count sheet again: %v", err)
	}
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet again: %v", err)
	}
	if err := expect.Locator(search).ToBeVisible(); err != nil {
		t.Errorf("the + doesn't open the Count sheet on its search: %v", err)
	}
	if err := expect.Locator(stated).ToBeHidden(); err != nil {
		t.Errorf("the Count sheet still shows the Stock Item's form: %v", err)
	}
	if err := expect.Locator(sheet).Not().ToContainText("Counted"); err != nil {
		t.Errorf("the Count sheet still confirms the last Count: %v", err)
	}
}

// A search finding nothing opens the Ingredients sheet over the Count sheet, started on
// what was typed. Adding the Ingredient closes that sheet, since the User came to count
// it, and leaves them on its Count, ready for the quantity.
func TestBrowserAddsAnIngredientFromTheCountSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	expect := playwright.NewPlaywrightAssertions()
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	count := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	if err := count.GetByRole("searchbox").Fill("salmon"); err != nil {
		t.Fatalf("search: %v", err)
	}
	if err := count.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Add salmon as a new Ingredient"}).Click(); err != nil {
		t.Fatalf("offer to add salmon: %v", err)
	}

	ings := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	if err := expect.Locator(ings.GetByLabel("Name", playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(true)})).ToHaveValue("salmon"); err != nil {
		t.Fatalf("the add form isn't started on salmon: %v", err)
	}
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Errorf("the Ingredients sheet closed the Count sheet: %v", err)
	}
	fillsIngredient(t, ings, "salmon")

	if err := expect.Locator(ings).ToBeHidden(); err != nil {
		t.Errorf("adding to count leaves the Ingredients sheet open: %v", err)
	}
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Fatalf("adding to count closes the Count sheet: %v", err)
	}
	stated := count.GetByLabel("How much is there")
	if err := expect.Locator(stated).ToBeFocused(); err != nil {
		t.Errorf("adding to count doesn't focus the quantity: %v", err)
	}
	if err := stated.Fill("300"); err != nil {
		t.Fatalf("state the quantity: %v", err)
	}
	if err := count.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("save the Count: %v", err)
	}
	if err := expect.Locator(count).ToContainText("Counted salmon: 300 g in the pantry."); err != nil {
		t.Errorf("the new Ingredient's Count isn't confirmed: %v", err)
	}
}
