package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// A Stock Item's Movements answer why it holds what it does: newest first, each with when
// it happened, its cause, what Dandori believed before and after, and the User's note.
func TestAStockItemsMovementsTellWhyItHoldsWhatItDoes(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 09:00")
	s.Counts("rice", "", "200", "")
	s.At("Mon 14:05")
	s.Counts("rice", "", "700", "bought more")
	s.At("Tue 19:30")
	s.Counts("rice", "", "450", "")

	s.Open("/kitchen")
	s.Follows("rice")
	s.IsAt("/kitchen/movements")
	s.Sees("Movements")
	s.Sees("rice · Pantry")
	s.Sees("Counted 700 g → 450 g Tuesday 3 November, 19:30 " +
		"Counted 200 g → 700 g Monday 2 November, 14:05 bought more " +
		"Counted 0 g → 200 g Monday 2 November, 09:00")
}

// A mistaken Count is corrected by another Count, and both stay: the Movements offer no
// way to change or remove one. A Count that changes nothing is one of them too.
func TestAMistakeIsCorrectedByAnotherCount(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 10:00")
	s.Counts("rice", "", "450", "")
	s.At("Mon 10:01")
	s.Counts("rice", "", "4500", "")
	s.At("Mon 10:02")
	s.Counts("rice", "", "450", "a slip of the finger")
	s.At("Mon 18:00")
	s.Counts("rice", "", "450", "still there")

	s.Open("/kitchen")
	s.Sees("rice 450 g")
	s.Follows("rice")
	s.Sees("Counted 450 g → 450 g Monday 2 November, 18:00 still there " +
		"Counted 4500 g → 450 g Monday 2 November, 10:02 a slip of the finger " +
		"Counted 450 g → 4500 g Monday 2 November, 10:01 " +
		"Counted 0 g → 450 g Monday 2 November, 10:00")
	_, main, ok := strings.Cut(s.Page().Body, "<main")
	if !ok {
		t.Fatalf("the Movements page has no main content: %s", s.Page().Body)
	}
	if strings.Contains(main, "<form") {
		t.Errorf("the Movements offer a form: %s", apptest.Text(main))
	}
}

// A Stock Item is one Ingredient in one Storage Location: its Movements are its own.
func TestEachStockItemHasItsOwnMovements(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.Counts("rice", "", "700", "in the jar")
	s.Counts("rice", "freezer", "300", "cooked")
	s.Counts("egg", "", "6", "")

	s.Open("/kitchen")
	s.Follows("egg")
	s.Sees("egg · Fridge")
	s.Sees("Counted 0 pieces → 6 pieces")
	s.DoesNotSee("in the jar")

	s.Open("/kitchen")
	s.Follows("Count rice")
	s.Follows("Movements in the freezer")
	s.Sees("rice · Freezer")
	s.Sees("Counted 0 g → 300 g")
	s.Sees("cooked")
	s.DoesNotSee("in the jar")
}

// A Stock Item counted to nothing leaves the Kitchen page but keeps its Movements, which
// its Ingredient's Count form links, in whichever Storage Location it was.
func TestAStockItemAtNothingKeepsItsMovements(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "freezer", "300", "")
	s.Counts("rice", "freezer", "0", "it thawed")
	s.Open("/kitchen")
	s.DoesNotSee("Count rice")

	s.Open("/kitchen/count")
	s.Follows("rice")
	s.Follows("Movements in the freezer")
	s.IsAt("/kitchen/movements")
	s.Sees("Counted 300 g → 0 g")
	s.Sees("it thawed")
}

// The Count form links only the Stock Items that have Movements.
func TestACountFormLinksOnlyMovementsThereAre(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.Counts("rice", "", "700", "")

	s.Open("/kitchen/count")
	s.Follows("egg")
	s.DoesNotSee("Movements in the")

	s.Open("/kitchen/count")
	s.Follows("rice")
	s.Sees("Movements in the pantry")
	s.DoesNotSee("Movements in the fridge")
	s.DoesNotSee("Movements in the freezer")
}

// A Stock Item with no Movements isn't there to open, and neither is another User's.
func TestMovementsOfNoStockItem(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.Invite("bea@example.com")
	s.Counts("rice", "", "700", "keep out")
	riceID, eggID := ingredientID(t, s, "rice"), ingredientID(t, s, "egg")

	for _, query := range []string{
		"", "?ingredient=lots&location=pantry", "?ingredient=" + riceID + "&location=freezer",
		"?ingredient=" + riceID + "&location=cellar", "?ingredient=" + eggID + "&location=fridge",
	} {
		for _, path := range []string{"/kitchen/movements", "/kitchen/movements/sheet"} {
			if got := s.Get(t, path+query); got.StatusCode != http.StatusNotFound {
				t.Errorf("%s%s answered %d, want 404", path, query, got.StatusCode)
			}
		}
	}

	s.SwitchesTo("bea@example.com")
	s.ChoosesLanguage("English")
	for _, path := range []string{"/kitchen/movements", "/kitchen/movements/sheet"} {
		theirs := s.Get(t, path+"?ingredient="+riceID+"&location=pantry")
		if theirs.StatusCode != http.StatusNotFound || strings.Contains(theirs.Body, "keep out") {
			t.Errorf("%s of another User's Stock Item answered %d", path, theirs.StatusCode)
		}
	}
}

// A Movement of another year says so; one of this year has no need to.
func TestAMovementOfAnotherYearShowsItsYear(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 09:00")
	s.Counts("rice", "", "200", "")
	s.At("2027-01-04 10:00")
	s.Counts("rice", "", "700", "")

	s.Open("/kitchen")
	s.Follows("rice")
	s.Sees("Counted 200 g → 700 g Monday 4 January, 10:00 " +
		"Counted 0 g → 200 g Monday 2 November 2026, 09:00")
}

// The Movements speak the User's Language, whatever the Language of their note.
func TestMovementsInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "uovo", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6", Carbs: "0", Fat: "5"})
	s.At("Mon 14:05")
	s.Counts("uovo", "", "1", "dal vicino")

	s.Open("/kitchen")
	s.Follows("uovo")
	s.Sees("Movimenti")
	s.Sees("uovo · Frigo")
	s.Sees("Contato 0 pezzi → 1 pezzo")
	s.Sees("lunedì 2 novembre alle 14:05")
	s.Sees("dal vicino")
}

// Italian says "in" or "nel" as each Storage Location takes it, on a Count's confirmation
// and on the link to its Movements.
func TestStorageLocationsInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "uovo", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6", Carbs: "0", Fat: "5"})

	for location, in := range map[string]string{"pantry": "in dispensa", "fridge": "in frigo", "freezer": "nel congelatore"} {
		s.Counts("uovo", location, "2", "")
		s.Sees("Contato uovo: 2 pezzi " + in + ".")
		s.Open("/kitchen/count")
		s.Follows("uovo")
		s.Sees("Movimenti " + in)
	}
}

// As htmx asks for them, the Movements are the sheet's body alone.
func TestTheMovementsSheetIsAFragment(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "700", "")
	sheet := s.HTMXGet(t, "/kitchen/movements/sheet?location=pantry&ingredient="+ingredientID(t, s, "rice"))
	if sheet.StatusCode != http.StatusOK || strings.Contains(sheet.Body, "<html") {
		t.Fatalf("the sheet's body answered %d, want 200 and a fragment:\n%s", sheet.StatusCode, sheet.Body)
	}
	if shown := apptest.Text(sheet.Body); !strings.Contains(shown, "Counted 0 g → 700 g") {
		t.Errorf("the sheet shows: %s", shown)
	}
}

// Opening a Stock Item shows its Movements in the side sheet, newest first, leaving the
// User where they are on the Kitchen page; a Count form opens them over its sheet.
func TestBrowserOpensAStockItemsMovementsInPlace(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	addsRiceInTheBrowser(t, b)
	expect := playwright.NewPlaywrightAssertions()

	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	count := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	if err := count.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "rice"}).Click(); err != nil {
		t.Fatalf("choose rice: %v", err)
	}
	if err := count.GetByLabel("How much is there").Fill("700"); err != nil {
		t.Fatalf("state the quantity: %v", err)
	}
	if err := count.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("save the Count: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("rice"); err != nil {
		t.Fatalf("rice isn't listed: %v", err)
	}
	if err := count.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click(); err != nil {
		t.Fatalf("close the Count sheet: %v", err)
	}

	if err := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "rice", Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatalf("open rice: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Movements"})
	if err := expect.Locator(sheet).ToContainText("Counted"); err != nil {
		t.Fatalf("the Movements don't open: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("0 g → 700 g"); err != nil {
		t.Errorf("the Movements show the wrong Count: %v", err)
	}
	b.IsAt(t, "/kitchen")
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click(); err != nil {
		t.Fatalf("close the Movements: %v", err)
	}
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("the Movements don't close: %v", err)
	}

	// A Count form's link opens them over the Count sheet, which stays open beneath.
	if err := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("open the Stock Item's Count: %v", err)
	}
	if err := count.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Movements in the pantry"}).Click(); err != nil {
		t.Fatalf("open the Movements from the Count: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("0 g → 700 g"); err != nil {
		t.Fatalf("the Count form's link doesn't open the Movements: %v", err)
	}
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Errorf("the Movements closed the Count sheet: %v", err)
	}
}
