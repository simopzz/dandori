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
	s.Sees("Counted 700 g → 450 g Tue 3 Nov 19:30 " +
		"Counted 200 g → 700 g Mon 2 Nov 14:05 bought more " +
		"Counted 0 g → 200 g Mon 2 Nov 09:00")
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
	s.Sees("Counted 450 g → 450 g Mon 2 Nov 18:00 still there " +
		"Counted 4500 g → 450 g Mon 2 Nov 10:02 a slip of the finger " +
		"Counted 450 g → 4500 g Mon 2 Nov 10:01 " +
		"Counted 0 g → 450 g Mon 2 Nov 10:00")
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
	s.Follows("Movements")
	s.Sees("rice · Pantry")
	s.Follows("Freezer")
	s.Sees("rice · Freezer")
	s.Sees("Counted 0 g → 300 g")
	s.Sees("cooked")
	s.DoesNotSee("in the jar")
}

// A Stock Item counted to nothing leaves the Kitchen page but keeps its Movements, which
// its Ingredient's Count form reaches, in whichever Storage Location it was.
func TestAStockItemAtNothingKeepsItsMovements(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "freezer", "300", "")
	s.Counts("rice", "freezer", "0", "it thawed")
	s.Open("/kitchen")
	s.DoesNotSee("Count rice")

	s.Open("/kitchen/count")
	s.Follows("rice")
	s.Follows("Movements")
	s.Follows("Freezer")
	s.IsAt("/kitchen/movements")
	s.Sees("Counted 300 g → 0 g")
	s.Sees("it thawed")
}

// The Count form has one Movements link and one Choose another, whatever Movements its
// Ingredient has. Without htmx the link opens the Storage Location the form started on.
func TestACountFormHasOneMovementsLink(t *testing.T) {
	oats := rice
	oats.Name = "oats"
	s := kitchenWith(t, rice, egg, oats)
	s.Counts("oats", "", "500", "")
	s.Counts("rice", "", "700", "")
	s.Counts("rice", "freezer", "300", "")
	s.Moves("Move rice in the freezer", "100", "fridge", "")

	for _, form := range []struct{ name, query, heading string }{
		{"egg", "", "egg · Fridge"},
		{"oats", "", "oats · Pantry"},
		{"rice", "", "rice · Pantry"},
		{"rice", "&location=freezer", "rice · Freezer"},
	} {
		s.Open("/kitchen/count?ingredient=" + ingredientID(t, s, form.name) + form.query)
		body := s.Page().Body
		if links := strings.Count(body, `href="/kitchen/movements?`); links != 1 {
			t.Errorf("the Count form of %s has %d links to Movements, want 1", form.name, links)
		}
		if another := strings.Count(body, "Choose another"); another != 1 {
			t.Errorf("the Count form of %s has %d Choose another, want 1", form.name, another)
		}
		s.Follows("Movements")
		s.Sees(form.heading)
	}
}

// A Storage Location with no Movements of an Ingredient the User has says so, as a page
// and in the sheet. A Staple is such an Ingredient, though no Stock Item holds it.
func TestAStorageLocationWithNoMovementsSaysSo(t *testing.T) {
	s := kitchenWith(t, rice, salt)
	s.Counts("rice", "", "700", "")

	for _, empty := range []struct{ name, location, heading string }{
		{"rice", "freezer", "rice · Freezer"},
		{"salt", "pantry", "salt · Pantry"},
	} {
		query := "?ingredient=" + ingredientID(t, s, empty.name) + "&location=" + empty.location
		s.Open("/kitchen/movements" + query)
		s.Sees(empty.heading)
		s.Sees("No Movements here yet.")

		sheet := s.HTMXGet(t, "/kitchen/movements/sheet"+query)
		if shown := apptest.Text(sheet.Body); sheet.StatusCode != http.StatusOK ||
			!strings.Contains(shown, empty.heading) || !strings.Contains(shown, "No Movements here yet.") {
			t.Errorf("the sheet of %s answered %d and shows: %s", empty.heading, sheet.StatusCode, shown)
		}
	}

	s.Open("/kitchen/movements?location=pantry&ingredient=" + ingredientID(t, s, "rice"))
	s.DoesNotSee("No Movements here yet.")
}

// The Movements link all three Storage Locations, the current one marked, so without htmx
// each stays reachable and a move can be followed from one side to the other.
func TestMovementsLinkEveryStorageLocation(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "800", "")
	s.Moves("Move rice in the pantry", "300", "freezer", "")

	s.Open("/kitchen")
	s.Follows("rice")
	s.Sees("rice · Pantry")
	s.Sees("Moved to the freezer")
	if current := apptest.CurrentLink(t, s.Page().Body); current != "Pantry" {
		t.Errorf("the current Storage Location is %q, want Pantry", current)
	}

	s.Follows("Freezer")
	s.IsAt("/kitchen/movements")
	s.Sees("rice · Freezer")
	s.Sees("Moved from the pantry")
	if current := apptest.CurrentLink(t, s.Page().Body); current != "Freezer" {
		t.Errorf("the current Storage Location is %q, want Freezer", current)
	}

	s.Follows("Fridge")
	s.Sees("rice · Fridge")
	s.Sees("No Movements here yet.")

	sheet := s.HTMXGet(t, "/kitchen/movements/sheet?location=fridge&ingredient="+ingredientID(t, s, "rice"))
	if current := apptest.CurrentLink(t, sheet.Body); current != "Fridge" {
		t.Errorf("the sheet's current Storage Location is %q, want Fridge", current)
	}
	for _, location := range []string{"Pantry", "Fridge", "Freezer"} {
		apptest.Link(t, sheet.Body, location)
	}
}

// Only an Ingredient the User has, in a Storage Location, is there to open.
func TestMovementsOfNoStockItem(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Invite("bea@example.com")
	s.Counts("rice", "", "700", "keep out")
	riceID := ingredientID(t, s, "rice")

	for _, query := range []string{
		"", "?ingredient=lots&location=pantry", "?ingredient=" + riceID,
		"?ingredient=" + riceID + "&location=cellar", "?ingredient=0&location=pantry",
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
		for _, location := range []string{"pantry", "freezer"} {
			theirs := s.Get(t, path+"?ingredient="+riceID+"&location="+location)
			if theirs.StatusCode != http.StatusNotFound || strings.Contains(theirs.Body, "keep out") {
				t.Errorf("%s of another User's Stock Item in the %s answered %d", path, location, theirs.StatusCode)
			}
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
	s.Sees("Counted 200 g → 700 g Mon 4 Jan 10:00 " +
		"Counted 0 g → 200 g Mon 2 Nov 2026 09:00")
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
	s.Sees("Lun 2 Nov 14:05")
	s.Sees("dal vicino")

	s.Follows("Congelatore")
	s.Sees("uovo · Congelatore")
	s.Sees("Ancora nessun Movimento qui.")
}

// A Count's confirmation says "in" or "nel" as each Storage Location takes it.
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

	// A Count form's link opens the Storage Location chosen in the form at that moment,
	// over the Count sheet, which stays open beneath.
	if err := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("open the Stock Item's Count: %v", err)
	}
	movements := count.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Movements"})
	if err := movements.Click(); err != nil {
		t.Fatalf("open the Movements from the Count: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("0 g → 700 g"); err != nil {
		t.Fatalf("the Count form's link doesn't open the Movements: %v", err)
	}
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Errorf("the Movements closed the Count sheet: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close"}).Click(); err != nil {
		t.Fatalf("close the Movements: %v", err)
	}

	if err := count.GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: "fridge 0 g"}).Check(playwright.LocatorCheckOptions{Force: playwright.Bool(true)}); err != nil {
		t.Fatalf("choose the fridge: %v", err)
	}
	if err := movements.Click(); err != nil {
		t.Fatalf("open the fridge's Movements: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("rice · Fridge"); err != nil {
		t.Fatalf("the Movements don't follow the form: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("No Movements here yet."); err != nil {
		t.Errorf("the fridge doesn't say it has no Movements: %v", err)
	}
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Errorf("the Movements closed the Count sheet: %v", err)
	}
	b.Shot(t, "movements-empty")

	// The sheet's own links swap it in place and keep the focus in it.
	if err := sheet.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Pantry"}).Click(); err != nil {
		t.Fatalf("open the pantry's Movements: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("rice · Pantry"); err != nil {
		t.Fatalf("the sheet doesn't swap to the pantry: %v", err)
	}
	current := sheet.Locator("[aria-current=true]")
	if err := expect.Locator(current).ToHaveText("Pantry"); err != nil {
		t.Errorf("the pantry isn't marked current: %v", err)
	}
	if err := expect.Locator(current).ToBeFocused(); err != nil {
		t.Errorf("the swap lost the focus: %v", err)
	}
	if err := expect.Locator(count).ToBeVisible(); err != nil {
		t.Errorf("the swap closed the Count sheet: %v", err)
	}
	b.Shot(t, "movements-sheet")
}

// box is where the locator l of what stands on the screen.
func box(t *testing.T, what string, l playwright.Locator) *playwright.Rect {
	t.Helper()
	rect, err := l.BoundingBox()
	if err != nil || rect == nil {
		t.Fatalf("%s isn't on the screen: %v", what, err)
	}
	return rect
}

// On a phone, in Italian, the Count form keeps Choose another beside the Ingredient's
// name, and Movements across from the Count button, without overflowing the sheet.
func TestBrowserCountFormHoldsItsLayoutOnAPhoneInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "riso", Unit: "g", Usual: "pantry", Kcal: "350", Protein: "7", Carbs: "78", Fat: "0.6"})

	b := s.Browser(t)
	if err := b.SetViewportSize(360, 740); err != nil {
		t.Fatalf("size the page as a phone: %v", err)
	}
	b.SignIn(t, "ada@example.com")
	b.IsAt(t, "/")
	b.Open(t, "/kitchen")
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Conta"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Conta"})
	if err := sheet.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "riso"}).Click(); err != nil {
		t.Fatalf("choose riso: %v", err)
	}
	expect := playwright.NewPlaywrightAssertions()
	form := sheet.Locator("#count-form")
	another := form.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Scegline un altro"})
	movements := form.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Movimenti"})
	count := form.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Conta"})
	if err := expect.Locator(another).ToBeVisible(); err != nil {
		t.Fatalf("Choose another isn't shown: %v", err)
	}

	name := box(t, "the name", form.GetByText("riso", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}))
	chooser := box(t, "Choose another", another)
	if centre := chooser.Y + chooser.Height/2; centre < name.Y || centre > name.Y+name.Height {
		t.Errorf("Choose another isn't beside the name: %+v, %+v", chooser, name)
	}
	if first := box(t, "the first Storage Location", form.GetByRole("radio").First()); chooser.Y >= first.Y {
		t.Errorf("Choose another isn't above the form: %+v, %+v", chooser, first)
	}

	button, link := box(t, "the Count button", count), box(t, "Movements", movements)
	if centre := link.Y + link.Height/2; centre < button.Y || centre > button.Y+button.Height {
		t.Errorf("Movements isn't across from the button: %+v, %+v", link, button)
	}
	edge := box(t, "the form", form)
	if at := link.X + link.Width; at > edge.X+edge.Width+1 || at < edge.X+edge.Width-1 {
		t.Errorf("Movements ends at %v, not the form's edge %v", at, edge.X+edge.Width)
	}
	overflow, err := sheet.Evaluate(`s => s.scrollWidth > s.clientWidth`, nil)
	if err != nil || overflow != false {
		t.Errorf("the sheet overflows the phone: %v, %v", overflow, err)
	}
	b.Shot(t, "count-phone-italian")
}
