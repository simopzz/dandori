package app_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// Moving part of a Stock Item leaves the rest and adds to the other Storage Location's,
// the Stock Item it makes there if none was. Moving all of it removes the source, whose
// Movements stay.
func TestMovingStockBetweenStorageLocations(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 09:00")
	s.Counts("rice", "", "800", "")

	s.At("Mon 10:00")
	s.Moves("Move rice in the pantry", "300", "freezer", "for next week")
	s.IsAt("/kitchen")
	s.IsConfirmed("Moved 300 g of rice to the freezer.")
	s.Sees("Pantry rice 500 g")
	s.Sees("Freezer rice 300 g")

	s.At("Mon 11:00")
	s.Moves("Move rice in the pantry", "", "freezer", "")
	s.IsConfirmed("Moved 500 g of rice to the freezer.")
	s.DoesNotSee("Pantry")

	s.Open("/kitchen")
	s.IsNotConfirmed()
	s.Sees("Freezer rice 800 g")

	s.Follows("rice")
	s.Sees("rice · Freezer")
	s.Sees("Moved from the pantry 300 g → 800 g Mon 2 Nov 11:00 " +
		"Moved from the pantry 0 g → 300 g Mon 2 Nov 10:00 for next week")

	s.Open("/kitchen/count")
	s.Follows("rice")
	s.Follows("Movements")
	s.Sees("rice · Pantry")
	s.Sees("Moved to the freezer 500 g → 0 g Mon 2 Nov 11:00 " +
		"Moved to the freezer 800 g → 500 g Mon 2 Nov 10:00 for next week " +
		"Counted 0 g → 800 g Mon 2 Nov 09:00")
}

// The form starts from all of the Stock Item, in its unit, and leaves the target to the
// User, offering the two other Storage Locations with what Dandori believes they hold.
func TestAMoveStartsFromAllOfTheStockItem(t *testing.T) {
	s := kitchenWith(t, egg)
	s.Counts("egg", "", "1.5", "")
	s.Counts("egg", "freezer", "4", "")

	s.Open("/kitchen")
	s.Follows("Move egg in the fridge")
	s.IsAt("/kitchen/move")
	s.Sees("egg · Fridge")
	s.Sees("pantry 0 pieces")
	s.Sees("freezer 4 pieces")
	s.DoesNotSee("fridge 1.5 pieces")
	form := apptest.Form(t, s.Page().Body, "move-form")
	if got := form.Get("quantity"); got != "1.5" {
		t.Errorf("the quantity starts at %q, want all of it", got)
	}
	if got := form.Get("to"); got != "" {
		t.Errorf("the target starts at %q, want none", got)
	}

	s.Moves("Move egg in the fridge", "", "pantry", "")
	s.Sees("Pantry egg 1.5 pieces")
	s.Sees("Freezer egg 4 pieces")
	s.DoesNotSee("Fridge")
}

// A quantity follows its unit's rules: pieces have one decimal place.
func TestMovingPartOfAPiece(t *testing.T) {
	s := kitchenWith(t, egg)
	s.Counts("egg", "", "2", "")
	s.Moves("Move egg in the fridge", "0.5", "freezer", "")
	s.Sees("Fridge egg 1.5 pieces")
	s.Sees("Freezer egg 0.5 pieces")
}

// A move is refused on the field, keeping what was typed and leaving the
// kitchen as it was.
func TestARefusedMove(t *testing.T) {
	cases := []struct {
		name, quantity, why string
	}{
		{"more than is held", "801", "Dandori believes only 800 g is here. If there is more, count it first."},
		{"nothing", "0", "Say how much to move."},
		{"nothing typed", " ", "Say how much to move."},
		{"below nothing", "-1", "Can't be below 0."},
		{"part of a gram", "12.5", "Grams and millilitres are whole numbers."},
		{"not a number", "lots", "Type a number."},
		{"too much", "1000000", "Must be below 1000000."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := kitchenWith(t, rice)
			s.Counts("rice", "", "800", "")
			s.Moves("Move rice in the pantry", c.quantity, "freezer", "a note")
			s.IsRefused()
			s.Sees(c.why)
			s.IsNotConfirmed()
			form := apptest.Form(t, s.Page().Body, "move-form")
			if form.Get("to") != "freezer" || form.Get("note") != "a note" || form.Get("quantity") != strings.TrimSpace(c.quantity) {
				t.Errorf("the refused move holds %q to %q with note %q, want what was typed", form.Get("quantity"), form.Get("to"), form.Get("note"))
			}
			s.Open("/kitchen")
			s.Sees("Pantry rice 800 g")
			s.DoesNotSee("Freezer")
		})
	}
}

// When Dandori holds too little, the User counts first and the same move
// then goes through.
func TestCountingFirstLetsAMoveThrough(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "800", "")
	s.Moves("Move rice in the pantry", "1000", "freezer", "")
	s.IsRefused()

	s.Counts("rice", "pantry", "1200", "more than I thought")
	s.Moves("Move rice in the pantry", "1000", "freezer", "")
	s.Sees("Pantry rice 200 g")
	s.Sees("Freezer rice 1000 g")
}

// A move needs a target, which is another Storage Location.
func TestAMoveNeedsAnotherStorageLocation(t *testing.T) {
	for name, to := range map[string]string{"none": "", "not one": "cellar", "where it is": "pantry"} {
		t.Run(name, func(t *testing.T) {
			s := kitchenWith(t, rice)
			s.Counts("rice", "", "800", "")
			s.Moves("Move rice in the pantry", "300", to, "")
			s.IsRefused()
			s.Sees("Choose where it goes.")
			if got := apptest.Form(t, s.Page().Body, "move-form").Get("quantity"); got != "300" {
				t.Errorf("the refused move holds %q, want what was typed", got)
			}
			s.Open("/kitchen")
			s.Sees("Pantry rice 800 g")
		})
	}
}

// A move is checked against the Kitchen Ledger as it stands, not as the form showed it.
func TestAMoveFromAnOldFormIsChecked(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "800", "")
	s.Open("/kitchen")
	s.Follows("Move rice in the pantry")
	form := apptest.Form(t, s.Page().Body, "move-form")

	s.Moves("Move rice in the pantry", "", "fridge", "")

	form.Values.Set("to", "freezer")
	refused := s.PostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apptest.Text(refused.Body), "Dandori believes only 0 g is here.") {
		t.Errorf("the old form answered %d: %s", refused.StatusCode, apptest.Text(refused.Body))
	}
	s.Open("/kitchen")
	s.Sees("Fridge rice 800 g")
	s.DoesNotSee("Freezer")
}

// The Kitchen page says when a Stock Item was counted, which a move doesn't change: stock
// only moved somewhere was never counted there, and says nothing.
func TestAMoveIsNotACount(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 09:00")
	s.Counts("rice", "", "800", "")
	s.At("Mon 10:00")
	s.Moves("Move rice in the pantry", "300", "freezer", "")
	s.Sees("Pantry rice 500 g counted at 09:00")
	s.Sees("Freezer rice 300 g Count")
	s.DoesNotSee("0001")

	s.At("Mon 11:00")
	s.Counts("rice", "freezer", "300", "")
	s.Open("/kitchen")
	s.Sees("Freezer rice 300 g counted at 11:00")
}

// A Count of stock moved in starts from what the move left there.
func TestACountStartsFromWhatAMoveLeft(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "800", "")
	s.Moves("Move rice in the pantry", "300", "freezer", "")
	s.Counts("rice", "freezer", "250", "")

	s.Open("/kitchen/count")
	s.Follows("rice")
	s.Follows("Movements")
	s.Follows("Freezer")
	s.Sees("Counted 300 g → 250 g")
}

// A Stock Item holding nothing can't be moved, and neither can another User's. A post for
// a Storage Location that isn't one is no move at all.
func TestMoveOfNoStockItem(t *testing.T) {
	s := kitchenWith(t, rice, egg)
	s.Invite("bea@example.com")
	s.Counts("rice", "", "700", "")
	riceID, eggID := ingredientID(t, s, "rice"), ingredientID(t, s, "egg")

	for _, query := range []string{
		"", "?ingredient=lots&location=pantry", "?ingredient=" + riceID + "&location=freezer",
		"?ingredient=" + riceID + "&location=cellar", "?ingredient=" + eggID + "&location=fridge",
	} {
		for _, path := range []string{"/kitchen/move", "/kitchen/move/sheet"} {
			if got := s.Get(t, path+query); got.StatusCode != http.StatusNotFound {
				t.Errorf("%s%s answered %d, want 404", path, query, got.StatusCode)
			}
		}
	}
	post := url.Values{"ingredient": {riceID}, "location": {"cellar"}, "quantity": {"100"}, "to": {"freezer"}}
	if got := s.PostForm(t, "/kitchen/move", post); got.StatusCode != http.StatusNotFound {
		t.Errorf("moving from a cellar answered %d, want 404", got.StatusCode)
	}

	s.SwitchesTo("bea@example.com")
	s.ChoosesLanguage("English")
	if got := s.Get(t, "/kitchen/move?ingredient="+riceID+"&location=pantry"); got.StatusCode != http.StatusNotFound {
		t.Errorf("another User's Stock Item answered %d, want 404", got.StatusCode)
	}
	post.Set("location", "pantry")
	if got := s.PostForm(t, "/kitchen/move", post); got.StatusCode != http.StatusNotFound {
		t.Errorf("moving another User's Stock Item answered %d, want 404", got.StatusCode)
	}
}

// A move's cause speaks the User's Language, each Storage Location with its
// own preposition.
func TestMovesInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	s.AddsIngredient(apptest.Ingredient{Name: "uovo", Unit: "piece", Usual: "fridge", Kcal: "70", Protein: "6", Carbs: "0", Fat: "5"})
	s.Counts("uovo", "", "6", "")

	s.Moves("Sposta uovo in frigo", "2", "freezer", "")
	s.IsConfirmed("Spostato: 2 pezzi di uovo nel congelatore.")
	s.Moves("Sposta uovo nel congelatore", "1", "pantry", "")
	s.Moves("Sposta uovo in dispensa", "9", "fridge", "")
	s.IsRefused()
	s.Sees("Dandori crede che qui ce ne sia solo 1 pezzo.")
	s.Moves("Sposta uovo in dispensa", "", "fridge", "")

	s.Follows("uovo")
	s.Sees("Spostato dalla dispensa 4 pezzi → 5 pezzi")
	s.Sees("Spostato nel congelatore 6 pezzi → 4 pezzi")

	s.Open("/kitchen/count")
	s.Follows("uovo")
	s.Follows("Movimenti")
	s.Follows("Congelatore")
	s.Sees("Spostato in dispensa 2 pezzi → 1 pezzo")
	s.Sees("Spostato dal frigo 0 pezzi → 2 pezzi")
}

// As htmx posts it, the form answers with a fragment: the move confirmed in a floating
// Notice alone, closing the sheet and telling the page to list its stock again, or the
// form itself as typed with why it was refused, confirming nothing.
func TestAMovePostedAsHTMX(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "800", "")
	sheet := s.HTMXGet(t, "/kitchen/move/sheet?location=pantry&ingredient="+ingredientID(t, s, "rice"))
	if sheet.StatusCode != http.StatusOK || strings.Contains(sheet.Body, "<html") {
		t.Fatalf("the sheet's body answered %d, want 200 and a fragment:\n%s", sheet.StatusCode, sheet.Body)
	}
	form := apptest.Form(t, sheet.Body, "move-form")
	form.Values.Set("to", "freezer")

	form.Values.Set("quantity", "900")
	refused := s.HTMXPostForm(t, form.Action, form.Values)
	if refused.StatusCode != http.StatusUnprocessableEntity || strings.Contains(refused.Body, "<html") {
		t.Fatalf("the refused move answered %d, want 422 and a fragment:\n%s", refused.StatusCode, refused.Body)
	}
	if shown := apptest.Text(refused.Body); !strings.Contains(shown, "Dandori believes only 800 g is here.") {
		t.Errorf("the refused move shows: %s", shown)
	}
	if got := apptest.Form(t, refused.Body, "move-form").Get("quantity"); got != "900" {
		t.Errorf("the refused move holds %q, want what was typed", got)
	}
	if notices := apptest.Notices(t, refused.Body); len(notices) > 0 {
		t.Errorf("the refused move confirms %q", notices)
	}

	form.Values.Set("quantity", "300")
	moved := s.HTMXPostForm(t, form.Action, form.Values)
	if moved.StatusCode != http.StatusOK || strings.Contains(moved.Body, "<html") {
		t.Fatalf("the move answered %d, want 200 and a fragment:\n%s", moved.StatusCode, moved.Body)
	}
	if notices := apptest.Notices(t, moved.Body); !slices.Equal(notices, []string{"Moved 300 g of rice to the freezer."}) {
		t.Errorf("the move confirms %q", notices)
	}
	if strings.Contains(moved.Body, "<form") {
		t.Errorf("the confirmed move still offers a form: %s", moved.Body)
	}
	if got := moved.Header.Get("HX-Trigger"); got != "movement-saved, close-sheet" {
		t.Errorf("the move triggers %q, want the stock listed again and the sheet closed", got)
	}
	s.Open("/kitchen")
	s.Sees("Pantry rice 500 g")
	s.Sees("Freezer rice 300 g")
}

// A move from a row happens in its sheet: once saved the sheet closes, the list shows
// both Stock Items, and a floating Notice says what moved. A refused one keeps the sheet
// open on the field, confirming nothing.
func TestBrowserMovesStockInPlace(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	addsRiceInTheBrowser(t, b)
	expect := playwright.NewPlaywrightAssertions()

	count := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	if err := count.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "rice"}).Click(); err != nil {
		t.Fatalf("choose rice: %v", err)
	}
	if err := count.GetByLabel("How much is there").Fill("800"); err != nil {
		t.Fatalf("state the quantity: %v", err)
	}
	if err := count.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("save the Count: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("800 g"); err != nil {
		t.Fatalf("rice isn't listed: %v", err)
	}
	if err := count.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close", Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatalf("close the Count sheet: %v", err)
	}

	if err := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Move rice in the pantry"}).Click(); err != nil {
		t.Fatalf("open the move: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Move"})
	quantity := sheet.GetByLabel("How much to move")
	if err := expect.Locator(quantity).ToHaveValue("800"); err != nil {
		t.Fatalf("the quantity doesn't start at all of it: %v", err)
	}
	if err := sheet.GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: "Freezer 0 g"}).Check(); err != nil {
		t.Fatalf("choose the freezer: %v", err)
	}

	if err := quantity.Fill("900"); err != nil {
		t.Fatalf("type too much: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Move"}).Click(); err != nil {
		t.Fatalf("save the move: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("Dandori believes only 800 g is here."); err != nil {
		t.Fatalf("the refusal isn't on the field: %v", err)
	}
	if err := expect.Locator(b.Notices().Filter(playwright.LocatorFilterOptions{HasText: "Moved"})).ToHaveCount(0); err != nil {
		t.Errorf("the refused move is confirmed: %v", err)
	}
	b.Shot(t, "move-refused")

	if err := quantity.Fill("300"); err != nil {
		t.Fatalf("type less: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Move"}).Click(); err != nil {
		t.Fatalf("save the move: %v", err)
	}
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("the Move sheet stays open after the move: %v", err)
	}
	b.IsConfirmed(t, "Moved 300 g of rice to the freezer.")
	in := func(location string) playwright.Locator {
		return b.Locator("#stock .section").Filter(playwright.LocatorFilterOptions{HasText: location})
	}
	if err := expect.Locator(in("Pantry")).ToContainText("rice 500 g"); err != nil {
		t.Errorf("the list doesn't show what is left: %v", err)
	}
	if err := expect.Locator(in("Freezer")).ToContainText("rice 300 g"); err != nil {
		t.Errorf("the list doesn't show what moved: %v", err)
	}
	b.IsAt(t, "/kitchen")
	b.Shot(t, "move-confirmed")
}
