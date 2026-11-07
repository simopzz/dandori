package app_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// countsRiceInTheBrowser adds rice and counts 700 g of it in the pantry from the Kitchen
// page, closing each sheet, so the page lists its Stock Item.
func countsRiceInTheBrowser(t *testing.T, b *apptest.Browser) {
	t.Helper()
	addsRiceInTheBrowser(t, b)
	expect := playwright.NewPlaywrightAssertions()
	opensTheCountSheet(t, b)
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
	if err := expect.Locator(count).ToBeHidden(); err != nil {
		t.Fatalf("the Count sheet doesn't close: %v", err)
	}
}

// opensTheCountSheet presses the Kitchen page's +.
func opensTheCountSheet(t *testing.T, b *apptest.Browser) {
	t.Helper()
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	if err := playwright.NewPlaywrightAssertions().Locator(b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})).ToBeVisible(); err != nil {
		t.Fatalf("the Count sheet doesn't open: %v", err)
	}
}

// tapsBehind presses the screen where l stands, whatever is over it, as a finger does,
// and fails the test if the tap clicks anything outside a sheet: a press on the page.
func tapsBehind(t *testing.T, b *apptest.Browser, what string, l playwright.Locator) {
	t.Helper()
	r := box(t, what, l)
	if _, err := b.Evaluate(`document.addEventListener("click", (e) => { window.pressedBehind = !e.target.closest(".sheet") }, { capture: true, once: true })`); err != nil {
		t.Fatalf("listen for the tap: %v", err)
	}
	if err := b.Mouse().Click(r.X+r.Width/2, r.Y+r.Height/2); err != nil {
		t.Fatalf("tap %s: %v", what, err)
	}
	if pressed, err := b.Evaluate(`window.pressedBehind`); err != nil || pressed != false {
		t.Errorf("tapping %s behind the sheet presses the page (pressed %v, err %v)", what, pressed, err)
	}
}

// A tap on the page behind an open sheet closes the sheet and presses nothing there: not
// the link to the Ingredients, nor the + that opened it.
func TestBrowserATapBehindASheetClosesItAndPressesNothing(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	expect := playwright.NewPlaywrightAssertions()
	count := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	ings := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})

	opensTheCountSheet(t, b)
	tapsBehind(t, b, "the link to the Ingredients", b.Locator("main .lede a"))
	if err := expect.Locator(count).ToBeHidden(); err != nil {
		t.Errorf("a tap behind the Count sheet doesn't close it: %v", err)
	}
	if err := expect.Locator(ings).ToBeHidden(); err != nil {
		t.Errorf("a tap behind the Count sheet opens the Ingredients: %v", err)
	}

	opensTheCountSheet(t, b)
	tapsBehind(t, b, "the +", b.Locator(".fab"))
	if err := expect.Locator(count).ToBeHidden(); err != nil {
		t.Errorf("a tap on the + behind the Count sheet leaves it open: %v", err)
	}
	b.IsAt(t, "/kitchen")
}

// With the Movements sheet open over the Count sheet, a tap outside them closes the
// Movements alone, and so does Esc: each closes the top sheet only.
func TestBrowserATapOutsideOrEscClosesTheTopSheetOnly(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	countsRiceInTheBrowser(t, b)
	expect := playwright.NewPlaywrightAssertions()
	count := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	movements := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Movements"})
	opensTheMovementsOverTheCount := func() {
		t.Helper()
		if err := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
			t.Fatalf("open the Stock Item's Count: %v", err)
		}
		if err := count.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Movements"}).Click(); err != nil {
			t.Fatalf("open the Movements from the Count: %v", err)
		}
		if err := expect.Locator(movements).ToContainText("0 g → 700 g"); err != nil {
			t.Fatalf("the Movements don't open: %v", err)
		}
	}
	closesTheMovementsAlone := func(how string) {
		t.Helper()
		if err := expect.Locator(movements).ToBeHidden(); err != nil {
			t.Errorf("%s doesn't close the Movements: %v", how, err)
		}
		if err := expect.Locator(count).ToBeVisible(); err != nil {
			t.Errorf("%s closes the Count sheet under the Movements: %v", how, err)
		}
	}

	// The screen's left edge, halfway down, is outside both sheets on a desktop: the
	// Count centred, the Movements from the right.
	tapsTheVeil := func() {
		t.Helper()
		if err := b.Mouse().Click(8, 360); err != nil {
			t.Fatalf("tap the veil: %v", err)
		}
	}

	opensTheMovementsOverTheCount()
	tapsTheVeil()
	closesTheMovementsAlone("a tap outside")
	tapsTheVeil()
	if err := expect.Locator(count).ToBeHidden(); err != nil {
		t.Errorf("a second tap outside doesn't close the Count sheet: %v", err)
	}

	opensTheMovementsOverTheCount()
	if err := b.Keyboard().Press("Escape"); err != nil {
		t.Fatalf("press Esc: %v", err)
	}
	closesTheMovementsAlone("Esc")
}

// With a sheet open, Tab goes round its controls and never reaches the page behind it:
// past the last, focus may leave the document for the browser's own, then comes back.
func TestBrowserTabStaysInAnOpenSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	countsRiceInTheBrowser(t, b)
	if err := b.Locator("#stock").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("open the Stock Item's Count: %v", err)
	}
	stated := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"}).GetByLabel("How much is there")
	if err := playwright.NewPlaywrightAssertions().Locator(stated).ToBeFocused(); err != nil {
		t.Fatalf("the Count sheet doesn't open on its quantity: %v", err)
	}
	found, err := b.Evaluate(`document.getElementById("count").querySelectorAll("a, button, input, select, textarea").length`)
	controls, ok := found.(int)
	if err != nil || !ok {
		t.Fatalf("count the sheet's controls: %v, %v", found, err)
	}
	inSheet := 0
	for range 2*controls + 2 {
		if err := b.Keyboard().Press("Tab"); err != nil {
			t.Fatalf("press Tab: %v", err)
		}
		where, err := b.Evaluate(`(() => { const a = document.activeElement; return !a || a === document.body ? "nowhere" : a.closest("#count") ? "sheet" : a.outerHTML.slice(0, 80) })()`)
		if err != nil {
			t.Fatalf("find the focus: %v", err)
		}
		switch where {
		case "sheet":
			inSheet++
		case "nowhere":
		default:
			t.Fatalf("Tab leaves the sheet for %v", where)
		}
	}
	if inSheet <= controls {
		t.Errorf("Tab doesn't come back round the sheet: %d of its stops in it, for %v controls", inSheet, controls)
	}
}

// On a phone, scrolling over an open sheet taller than the screen scrolls the sheet to
// its end and leaves the page behind it where it was.
func TestBrowserScrollingAnOpenSheetLeavesThePageStill(t *testing.T) {
	var ings []apptest.Ingredient
	for _, name := range []string{"rice", "oats", "lentils", "flour", "sugar", "butter", "milk", "apples", "carrots", "onions", "garlic", "tomatoes", "chickpeas", "honey"} {
		ings = append(ings, apptest.Ingredient{Name: name, Unit: "g", Usual: "pantry", Kcal: "100", Protein: "1", Carbs: "1", Fat: "1"})
	}
	s := kitchenWith(t, ings...)
	for _, ing := range ings {
		s.Counts(ing.Name, "", "500", "")
	}
	b := s.Browser(t)
	if err := b.SetViewportSize(360, 640); err != nil {
		t.Fatalf("size the page as a phone: %v", err)
	}
	b.SignIn(t, "ada@example.com")
	b.IsAt(t, "/")
	b.Open(t, "/kitchen")
	if tall, err := b.Evaluate(`document.documentElement.scrollHeight > window.innerHeight`); err != nil || tall != true {
		t.Fatalf("the Kitchen page isn't taller than the phone (%v, %v)", tall, err)
	}
	if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredients"}).First().Click(); err != nil {
		t.Fatalf("open the Ingredients: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	if err := playwright.NewPlaywrightAssertions().Locator(sheet).ToContainText("honey"); err != nil {
		t.Fatalf("the Ingredients sheet doesn't list them: %v", err)
	}
	if tall, err := sheet.Evaluate(`s => s.scrollHeight > s.clientHeight`, nil); err != nil || tall != true {
		t.Fatalf("the Ingredients sheet isn't taller than the phone (%v, %v)", tall, err)
	}

	r := box(t, "the sheet", sheet)
	if err := b.Mouse().Move(r.X+r.Width/2, r.Y+r.Height/2); err != nil {
		t.Fatalf("point at the sheet: %v", err)
	}
	for range 30 {
		if err := b.Mouse().Wheel(0, 400); err != nil {
			t.Fatalf("scroll: %v", err)
		}
	}
	if _, err := sheet.Evaluate(`s => new Promise((done) => { const end = () => s.scrollTop + s.clientHeight >= s.scrollHeight - 1 ? done() : requestAnimationFrame(end); end() })`, nil); err != nil {
		t.Errorf("the sheet doesn't scroll to its end: %v", err)
	}
	if _, err := b.Evaluate(`new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))`); err != nil {
		t.Fatalf("wait for the scroll to settle: %v", err)
	}
	if y, err := b.Evaluate(`window.scrollY`); err != nil || y != 0 {
		t.Errorf("scrolling the sheet moves the page behind it to %v (%v)", y, err)
	}
}
