package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

var (
	peas    = apptest.Ingredient{Name: "peas", Unit: "g", Usual: "freezer", Kcal: "81", Protein: "5.4", Carbs: "14", Fat: "0.4"}
	lasagne = apptest.Portions{Name: "lasagne", HowMany: "3", Location: "freezer", Kcal: "620", Protein: "32", Carbs: "58", Fat: "28", Fibre: "4", Sugar: "8"}
)

// Portions no Prep Session produced are added from the Kitchen page and show as one group
// under their Storage Location, above its Stock Items.
func TestAddedPortionsShowGroupedAboveTheStockItems(t *testing.T) {
	s := kitchenWith(t, peas)
	s.Counts("peas", "", "400", "")

	s.AddsPortions(lasagne)
	s.IsAt("/kitchen")
	s.IsConfirmed("Added lasagne × 3 to the freezer.")
	s.Sees("Freezer lasagne × 3 per Portion: 620 kcal protein 32 g carbs 58 g fat 28 g fibre 4 g sugar 8 g peas 400 g")
}

// Calories, protein, carbs and fat are required, each refused on its field with what was
// typed kept. Nothing is added.
func TestAddingPortionsRefusesAMissingValueOnItsField(t *testing.T) {
	s := kitchenWith(t)
	p := lasagne
	p.Kcal, p.Protein, p.Carbs, p.Fat = "", "", "", ""

	s.AddsPortions(p)
	s.IsRefused()
	s.Sees("Calories Required. Protein Required. Carbs Required. Fat Required.")
	if got := apptest.Form(t, s.Page().Body, "portions-form").Get("name"); got != "lasagne" {
		t.Errorf("the refused form holds the name %q", got)
	}
	s.Open("/kitchen")
	s.Sees("Nothing in your kitchen yet.")
}

// A name and a Storage Location are refused on their fields too.
func TestAddingPortionsRefusesNoNameAndNoStorageLocation(t *testing.T) {
	s := kitchenWith(t)
	p := lasagne
	p.Name, p.Location = "  ", ""

	s.AddsPortions(p)
	s.IsRefused()
	s.Sees("Give it a name.")
	s.Sees("Choose where it is kept.")
}

// Fibre and sugar may be unknown, and show as unknown: never 0.
func TestAddedPortionsShowUnknownFibreAndSugarAsUnknown(t *testing.T) {
	s := kitchenWith(t)
	p := lasagne
	p.Fibre, p.Sugar = "", ""

	s.AddsPortions(p)
	s.Sees("fat 28 g fibre – unknown sugar – unknown")
	s.DoesNotSee("fibre 0")
}

// How many is a whole number from 1 to 99.
func TestAddingPortionsTakesAWholeNumberOfAtLeastOne(t *testing.T) {
	for _, c := range []struct{ typed, refused string }{
		{"", "Say how many."},
		{"0", "Must be a whole number of at least 1."},
		{"-1", "Must be a whole number of at least 1."},
		{"2.5", "Must be a whole number of at least 1."},
		{"three", "Must be a whole number of at least 1."},
		{"100", "Must be at most 99."},
	} {
		t.Run(c.typed, func(t *testing.T) {
			s := kitchenWith(t)
			p := lasagne
			p.HowMany = c.typed
			s.AddsPortions(p)
			s.IsRefused()
			s.Sees(c.refused)
		})
	}

	s := kitchenWith(t)
	p := lasagne
	p.HowMany = "99"
	s.AddsPortions(p)
	s.Sees("lasagne × 99")
	p.Name, p.HowMany = "soup", "1"
	s.AddsPortions(p)
	s.Sees("soup × 1")
}

// Alike means the same name, ignoring case and surrounding space, the same values and the
// same Storage Location: anything else is another group.
func TestPortionsGroupOnlyWhenAlike(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	fridge := lasagne
	fridge.Location, fridge.HowMany = "fridge", "1"
	s.AddsPortions(fridge)
	s.Sees("Fridge lasagne × 1")
	s.Sees("Freezer lasagne × 3")

	lighter := lasagne
	lighter.Kcal, lighter.HowMany = "480", "2"
	s.AddsPortions(lighter)
	s.Sees("lasagne × 3 per Portion: 620 kcal")
	s.Sees("lasagne × 2 per Portion: 480 kcal")

	spelled := lasagne
	spelled.Name, spelled.HowMany = "  LASAGNE ", "1"
	s.AddsPortions(spelled)
	s.Sees("lasagne × 4 per Portion: 620 kcal")
}

// An added Portion's name is a Food Term, so it fixes the User's Language.
func TestAnAddedPortionFixesTheLanguage(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.IsRefused()
	s.Sees("Your kitchen already holds Ingredients, Recipes or Portions in this Language, so it stays.")
}

// Portions are told in the User's Language.
func TestAddingPortionsSpeaksItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")

	s.AddsPortions(lasagne)
	s.IsConfirmed("Aggiunto: lasagne × 3 nel congelatore.")
	s.Sees("Congelatore lasagne × 3 per Porzione: 620 kcal")
}

// Two Users never see each other's Portions.
func TestUsersSeeOnlyTheirOwnPortions(t *testing.T) {
	s := kitchenWith(t)
	s.AddsPortions(lasagne)

	s.Invite("bob@example.com")
	s.SwitchesTo("bob@example.com")
	s.ChoosesLanguage("English")
	s.Open("/kitchen")
	s.DoesNotSee("lasagne")
	s.Sees("Nothing in your kitchen yet.")

	s.SwitchesTo("ada@example.com")
	s.Open("/kitchen")
	s.Sees("lasagne × 3")
}

// Posted from the sheet by htmx, an add answers with a fragment: the form as typed when
// refused, and when added a Notice that closes the sheet and lists the kitchen again.
func TestAddingPortionsAnswersHTMXWithAFragment(t *testing.T) {
	s := kitchenWith(t)
	s.Open("/kitchen/portions")
	form := apptest.Form(t, s.Page().Body, "portions-form")
	for field, v := range map[string]string{
		"name": "lasagne", "location": "freezer", "kcal": "620", "protein": "32", "carbs": "58", "fat": "28",
	} {
		form.Values.Set(field, v)
	}

	form.Values.Set("how_many", "100")
	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("the refused add answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if got := apptest.Form(t, refused.Body, "portions-form").Get("how_many"); got != "100" {
		t.Errorf("the refused add holds %q, want what was typed", got)
	}

	form.Values.Set("how_many", "3")
	added := s.HTMXPostForm(t, form.Action, form.Values)
	if added.StatusCode != http.StatusOK || strings.Contains(added.Body, "<html") {
		t.Fatalf("the add answered %d, want 200 and a fragment:\n%s", added.StatusCode, added.Body)
	}
	if notices := apptest.Notices(t, added.Body); !slices.Equal(notices, []string{"Added lasagne × 3 to the freezer."}) {
		t.Errorf("the add confirms %q", notices)
	}
	if got := added.Header.Get("HX-Trigger"); got != "movement-saved, close-sheet" {
		t.Errorf("the add triggers %q, want the kitchen listed again and the sheet closed", got)
	}
}

// The chevron only chooses what the + adds, and the + opens that sheet. A refused add
// comes back in it with focus on the refused field; one accepted closes it, shows the
// group and leaves the choice, which a new page forgets.
func TestBrowserAddsPortionsInTheSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	expect := playwright.NewPlaywrightAssertions()

	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Choose what to add"}).Click(); err != nil {
		t.Fatalf("open what to add: %v", err)
	}
	choice := b.GetByRole("radio", playwright.PageGetByRoleOptions{Name: "Add Portions"})
	if err := choice.Click(); err != nil {
		t.Fatalf("choose Portions: %v", err)
	}
	if err := expect.Locator(choice).ToBeHidden(); err != nil {
		t.Errorf("choosing doesn't close the menu: %v", err)
	}
	count := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count", Exact: playwright.Bool(true)})
	if err := expect.Locator(count).ToBeHidden(); err != nil {
		t.Errorf("the + still counts: %v", err)
	}
	plus := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Add Portions"})
	if err := expect.Locator(plus).ToBeFocused(); err != nil {
		t.Errorf("choosing doesn't leave focus on the +: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Add Portions"})
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("choosing opens the sheet by itself: %v", err)
	}
	b.Shot(t, "add-choice")
	if err := plus.Click(); err != nil {
		t.Fatalf("open the Add Portions sheet: %v", err)
	}
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Fatalf("the Add Portions sheet doesn't open: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("Name")).ToBeFocused(); err != nil {
		t.Errorf("opening the sheet doesn't focus its first field: %v", err)
	}
	b.Shot(t, "add-portions-sheet")

	fill := func(label, value string) {
		t.Helper()
		if err := sheet.GetByLabel(label).Fill(value); err != nil {
			t.Fatalf("fill %s: %v", label, err)
		}
	}
	fill("Name", "lasagne")
	fill("How many", "100")
	if err := sheet.GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: "freezer"}).Click(); err != nil {
		t.Fatalf("choose the freezer: %v", err)
	}
	fill("Calories", "620")
	fill("Protein", "32")
	fill("Carbs", "58")
	fill("Fat", "28")
	add := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add"})
	if err := add.Click(); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("Must be at most 99."); err != nil {
		t.Fatalf("the sheet doesn't refuse 100: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("How many")).ToBeFocused(); err != nil {
		t.Errorf("the refused field isn't focused: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("Name")).ToHaveValue("lasagne"); err != nil {
		t.Errorf("the refused form loses what was typed: %v", err)
	}

	fill("How many", "3")
	if err := add.Click(); err != nil {
		t.Fatalf("add again: %v", err)
	}
	b.IsConfirmed(t, "Added lasagne × 3 to the freezer.")
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("the sheet doesn't close: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("lasagne"); err != nil {
		t.Errorf("the group isn't listed: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("× 3"); err != nil {
		t.Errorf("the group isn't counted: %v", err)
	}
	b.IsAt(t, "/kitchen")
	b.Shot(t, "portions-listed")
	if err := expect.Locator(plus).ToBeVisible(); err != nil {
		t.Errorf("adding forgets what the + adds: %v", err)
	}

	b.Open(t, "/kitchen")
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Errorf("a new page doesn't start on Count: %v", err)
	}
	if err := expect.Locator(plus).ToBeHidden(); err != nil {
		t.Errorf("a new page remembers the choice: %v", err)
	}
}
