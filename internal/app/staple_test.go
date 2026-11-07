package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// Marking an Ingredient a Staple counts each of its Stock Items to nothing, the stock a
// move took to the freezer included, and says what was counted; they leave the Kitchen
// page, and each one's Movements tell why, keeping what Dandori believed.
func TestMarkingAStapleCountsItsStockToNothing(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 09:00")
	s.Counts("rice", "", "900", "")
	s.At("Mon 10:00")
	s.Moves("Move rice in the pantry", "200", "freezer", "")

	s.At("Mon 11:00")
	s.Open("/ingredients")
	s.EditsIngredient("rice", asStaple(rice))
	s.IsAt("/ingredients")
	s.IsConfirmed("700 g of rice in the pantry and 200 g in the freezer counted to nothing: it's a Staple now.")
	s.Sees("rice Staple pantry")

	s.Open("/kitchen")
	s.DoesNotSee("rice")

	s.Open("/ingredients")
	s.Follows("Movements")
	s.IsAt("/kitchen/movements")
	s.Sees("rice · Pantry")
	s.Sees("Marked a Staple 700 g → 0 g Mon 2 Nov 11:00 Moved to the freezer")
	s.Follows("Freezer")
	s.Sees("Marked a Staple 200 g → 0 g Mon 2 Nov 11:00 Moved from the pantry")
}

// asStaple is ing marked a Staple.
func asStaple(ing apptest.Ingredient) apptest.Ingredient {
	ing.Staple = true
	return ing
}

// Stock in one Storage Location is confirmed by its quantity alone.
func TestMarkingAStapleHeldInOneStorageLocation(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "700", "")
	s.Open("/ingredients")
	s.EditsIngredient("rice", asStaple(rice))
	s.IsConfirmed("700 g of rice counted to nothing: it's a Staple now.")
}

// Marking an Ingredient with no stock a Staple records no Movement: it has none to link
// to, and its unit still changes.
func TestMarkingAStapleWithNoStockRecordsNoMovement(t *testing.T) {
	s := kitchenWith(t, rice)
	s.EditsIngredient("rice", asStaple(rice))
	s.IsConfirmed("Saved rice.")
	s.Sees("rice Staple pantry")
	s.DoesNotSee("Movements")

	inPieces := asStaple(rice)
	inPieces.Unit = "piece"
	s.EditsIngredient("rice", inPieces)
	s.IsConfirmed("Saved rice.")
	s.Sees("rice Staple pantry per piece")
}

// A Staple counted to nothing and still a Staple is never counted again, nor moved by a
// post from an old form. Unmarked, it starts from nothing and is counted like any other.
func TestUnmarkingAStapleStartsItFromNothing(t *testing.T) {
	s := kitchenWith(t, rice)
	s.At("Mon 09:00")
	s.Counts("rice", "", "700", "")
	s.Open("/kitchen")
	s.Follows("Move rice in the pantry")
	move := apptest.Form(t, s.Page().Body, "move-form")

	s.At("Mon 10:00")
	s.Open("/ingredients")
	s.EditsIngredient("rice", asStaple(rice))
	s.Open("/kitchen/count?q=rice")
	s.Sees("rice is a Staple: it is never counted.")
	move.Values.Set("to", "freezer")
	if refused := s.PostForm(t, move.Action, move.Values); refused.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("moving a Staple from an old form answered %d: %s", refused.StatusCode, apptest.Text(refused.Body))
	}

	s.At("Mon 11:00")
	s.Open("/ingredients")
	s.EditsIngredient("rice", rice)
	s.IsConfirmed("Saved rice.")
	s.DoesNotSee("rice Staple")
	s.Open("/kitchen")
	s.Sees("Nothing in your kitchen yet.")

	s.Counts("rice", "", "300", "")
	s.IsConfirmed("Counted rice: 300 g in the pantry.")
	s.Open("/kitchen")
	s.Sees("Pantry rice 300 g")
	s.Follows("rice")
	s.Sees("Counted 0 g → 300 g Mon 2 Nov 11:00 Marked a Staple 700 g → 0 g Mon 2 Nov 10:00 Counted 0 g → 700 g")
}

// Posted from the side sheet by htmx, marking a Staple answers with the sheet's body,
// confirming what was counted, and tells the page under it that an Ingredient changed:
// TestBrowserMarksAStapleInTheSideSheet sees the Kitchen page list its stock again.
func TestTheIngredientsSheetMarksAStapleInPlace(t *testing.T) {
	s := kitchenWith(t, rice)
	s.Counts("rice", "", "700", "")
	edit := apptest.EditForm(t, s.HTMXGet(t, "/ingredients/sheet").Body, "rice")
	edit.Values.Set("staple", "on")
	marked := s.HTMXPostForm(t, edit.Action, edit.Values)
	if marked.StatusCode != http.StatusOK || strings.Contains(marked.Body, "<html") {
		t.Fatalf("marking rice from the sheet answered %d, want 200 and a fragment:\n%s", marked.StatusCode, marked.Body)
	}
	if notices := apptest.Notices(t, marked.Body); !slices.Equal(notices, []string{"700 g of rice counted to nothing: it's a Staple now."}) {
		t.Errorf("marking rice from the sheet confirms %q", notices)
	}
	if got := marked.Header.Get("HX-Trigger"); got != "ingredient-saved" {
		t.Errorf("marking rice from the sheet triggers %q, want ingredient-saved", got)
	}
}

// The Count to nothing a Staple makes, and the confirmation, speak Italian.
func TestMarkingAStapleInItalian(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("Italiano")
	s.Open("/ingredients")
	riso := apptest.Ingredient{Name: "riso", Unit: "g", Usual: "pantry", Kcal: "350", Protein: "7", Carbs: "78", Fat: "0.6"}
	s.AddsIngredient(riso)
	s.Counts("riso", "", "700", "")
	s.Moves("Sposta riso in dispensa", "200", "fridge", "")
	s.Open("/ingredients")
	s.EditsIngredient("riso", asStaple(riso))
	s.IsConfirmed("500 g di riso in dispensa e 200 g in frigo: contato a zero, ora è una Base.")
	s.Follows("Movimenti")
	s.Sees("Segnato come Base 500 g → 0 g")
}

// In the Ingredients sheet over the Kitchen page, marking an Ingredient a Staple confirms
// what was counted, and its Stock Item leaves the page under the sheet, still open.
func TestBrowserMarksAStapleInTheSideSheet(t *testing.T) {
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
	if err := count.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close", Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatalf("close the Count sheet: %v", err)
	}

	if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredients"}).First().Click(); err != nil {
		t.Fatalf("open the Ingredients: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	row := sheet.Locator("li.row").Filter(playwright.LocatorFilterOptions{HasText: "rice"})
	if err := row.Locator("summary").Click(); err != nil {
		t.Fatalf("open rice's row: %v", err)
	}
	if err := row.GetByLabel("Staple").Check(); err != nil {
		t.Fatalf("mark rice a Staple: %v", err)
	}
	if err := row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save"}).Click(); err != nil {
		t.Fatalf("save rice: %v", err)
	}
	b.IsConfirmed(t, "700 g of rice counted to nothing: it's a Staple now.")
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Errorf("the sheet closes on marking a Staple: %v", err)
	}
	if err := expect.Locator(b.Locator("#stock")).ToContainText("Nothing in your kitchen yet."); err != nil {
		t.Errorf("rice stays on the Kitchen page under the sheet: %v", err)
	}
	if err := row.Locator("summary").Click(); err != nil {
		t.Fatalf("open rice's row again: %v", err)
	}
	b.Shot(t, "staple-marked")
}
