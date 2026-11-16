package app_test

import (
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// TestPagesShowTheClocksDay through a real browser: the clock is set
// through the test-only switch.
func TestBrowserPagesShowTheClocksDay(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	tuesday := time.Date(2026, time.November, 3, 10, 0, 0, 0, h.Clock.Now().Location())

	b.SetClock(t, tuesday)
	b.Open(t, "/")
	b.Sees(t, "Tuesday, November 3")

	b.SetClock(t, tuesday.AddDate(0, 0, 1))
	b.Open(t, "/")
	b.Sees(t, "Wednesday, November 4")
}

// Signing in and out through a real browser, with htmx boosting the forms.
func TestBrowserSignsInByMagicLink(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)

	b.Open(t, "/")
	b.IsAt(t, "/sign-in")
	b.SignUp(t, "ada@example.com")
	b.Open(t, "/profile")
	b.Sees(t, "Signed in as ada@example.com")

	b.Click(t, "Sign out")
	b.IsAt(t, "/sign-in")
}

// Choosing a Language through a real browser: switched to Italian before signing in, the
// User keeps it at the choice, and every page follows.
func TestBrowserChoosesALanguage(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)

	b.Open(t, "/sign-in")
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Italiano"}).Click(); err != nil {
		t.Fatalf("switch to Italiano: %v", err)
	}
	b.Sees(t, "Accedi")

	if err := b.GetByLabel("Email").Fill("ada@example.com"); err != nil {
		t.Fatalf("type the email: %v", err)
	}
	b.Click(t, "Inviami un link di accesso")
	b.IsAt(t, "/sign-in/sent")
	if _, err := b.Goto(h.Auth.LastLink(t, "ada@example.com")); err != nil {
		t.Fatalf("open the magic link: %v", err)
	}
	b.Click(t, "Accedi a Dandori")
	b.IsAt(t, "/language")
	b.Sees(t, "Scegli la tua lingua")

	b.ChooseLanguage(t, "Italiano", "Continua")
	b.IsAt(t, "/")
	b.Sees(t, "lunedì 2 novembre")

	b.Open(t, "/profile")
	b.ChooseLanguage(t, "English", "Salva la Lingua")
	b.IsAt(t, "/profile")
	b.IsConfirmed(t, "Profile saved.")
}

// Today on a phone-sized screen: the Slot now, the day's later ones a click away, and
// nothing wider than the screen.
func TestBrowserTodayRendersOnAPhone(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	if err := b.SetViewportSize(390, 844); err != nil {
		t.Fatalf("size the page as a phone: %v", err)
	}
	b.SignUp(t, "ada@example.com")
	b.SetClock(t, time.Date(2026, time.November, 2, 13, 30, 0, 0, h.Clock.Now().Location()))

	b.Open(t, "/")
	b.Sees(t, "Now: lunch at 1:00 pm")
	later := b.GetByText("dinner", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})
	if visible, err := later.IsVisible(); err != nil || visible {
		t.Errorf("the day's later Slots show before a click (visible %v, err %v)", visible, err)
	}
	if err := b.GetByText("Later today").Click(); err != nil {
		t.Fatalf("open the day's later Slots: %v", err)
	}
	if err := later.WaitFor(); err != nil {
		t.Errorf("the day's later Slots don't show after a click: %v", err)
	}
	wide, err := b.Evaluate(`document.documentElement.scrollWidth > window.innerWidth`)
	if err != nil {
		t.Fatalf("measure the page: %v", err)
	}
	if wide == true {
		t.Error("Today is wider than a phone's screen")
	}
}

// The Ingredients side sheet opens from the Kitchen page, over it, focused on its search,
// its add form closed under "Add an Ingredient", and adds an Ingredient there: the sheet
// stays open, the Ingredient listed and the form cleared. An add refused shows why with
// focus on the field, the sheet still open and the page under it the same.
func TestBrowserAddsAnIngredientInTheSideSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredients"}).Click(); err != nil {
		t.Fatalf("open the Ingredients: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	add := sheet.Locator("#ingredient-add")
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Fatalf("the Ingredients sheet doesn't open: %v", err)
	}
	if err := expect.Locator(sheet.Locator(`#ingredient-search input[name="q"]`)).ToBeFocused(); err != nil {
		t.Errorf("opening the sheet doesn't focus its search: %v", err)
	}
	if err := expect.Locator(add).ToBeHidden(); err != nil {
		t.Errorf("opening the sheet shows its add form: %v", err)
	}
	b.IsAt(t, "/kitchen")

	fill := func(label, value string) {
		t.Helper()
		if err := add.GetByLabel(label, playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(true)}).Fill(value); err != nil {
			t.Fatalf("fill %s: %v", label, err)
		}
	}
	choose := func(label, option string) {
		t.Helper()
		if _, err := add.GetByLabel(label).SelectOption(playwright.SelectOptionValues{Labels: &[]string{option}}); err != nil {
			t.Fatalf("choose %s: %v", label, err)
		}
	}
	addEgg := func() {
		t.Helper()
		opensAddForm(t, sheet)
		fill("Name", "egg")
		choose("Unit", "piece")
		choose("Usually kept in", "Fridge")
		fill("Calories (kcal)", "70")
		fill("Protein (g)", "6.3")
		fill("Carbs (g)", "0.4")
		fill("Fat (g)", "4.8")
		if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add"}).Click(); err != nil {
			t.Fatalf("add egg: %v", err)
		}
	}

	addEgg()
	b.IsConfirmed(t, "Added egg.")
	if err := expect.Locator(sheet).ToContainText("per piece: 70 kcal"); err != nil {
		t.Errorf("the sheet doesn't list egg: %v", err)
	}
	if err := expect.Locator(add.GetByLabel("Name")).ToHaveValue(""); err != nil {
		t.Errorf("the add form isn't cleared after adding: %v", err)
	}
	b.IsAt(t, "/kitchen")

	addEgg()
	if err := expect.Locator(sheet).ToContainText("You already have egg"); err != nil {
		t.Fatalf("the sheet doesn't refuse egg again: %v", err)
	}
	if err := expect.Locator(add.GetByLabel("Name")).ToBeFocused(); err != nil {
		t.Errorf("the refused name isn't focused: %v", err)
	}
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Errorf("the sheet closes on a refused add: %v", err)
	}
	b.IsAt(t, "/kitchen")
	b.Sees(t, "Nothing in your kitchen yet.")

	b.Click(t, "Close")
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Errorf("the sheet doesn't close: %v", err)
	}
}

// In the side sheet, a near miss suggests the Ingredient it meant, and picking it adds
// nothing and finds it. The search ranks as the User types, keeping their focus even
// while a field of the add form is refused.
func TestBrowserSuggestsAndSearchesInTheSideSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredients"}).Click(); err != nil {
		t.Fatalf("open the Ingredients: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	add := sheet.Locator("#ingredient-add")
	expect := playwright.NewPlaywrightAssertions()
	name := add.GetByLabel("Name", playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(true)})
	addButton := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add"})
	sees := func(want string) {
		t.Helper()
		if err := expect.Locator(sheet).ToContainText(want); err != nil {
			t.Fatalf("the sheet doesn't show %q: %v", want, err)
		}
	}
	leavesName := func(value string) {
		t.Helper()
		opensAddForm(t, sheet)
		if err := name.Fill(value); err != nil {
			t.Fatalf("fill the name: %v", err)
		}
		if err := name.Press("Tab"); err != nil {
			t.Fatalf("leave the name: %v", err)
		}
	}
	fills := func(value string) {
		t.Helper()
		opensAddForm(t, sheet)
		for label, value := range map[string]string{"Calories (kcal)": "70", "Protein (g)": "6.3", "Carbs (g)": "0.4", "Fat (g)": "4.8"} {
			if err := add.GetByLabel(label, playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(true)}).Fill(value); err != nil {
				t.Fatalf("fill %s: %v", label, err)
			}
		}
		for label, option := range map[string]string{"Unit": "piece", "Usually kept in": "Fridge"} {
			if _, err := add.GetByLabel(label).SelectOption(playwright.SelectOptionValues{Labels: &[]string{option}}); err != nil {
				t.Fatalf("choose %s: %v", label, err)
			}
		}
		leavesName(value)
	}
	pressesAdd := func(options ...playwright.LocatorClickOptions) {
		t.Helper()
		if err := addButton.Click(options...); err != nil {
			t.Fatalf("press Add: %v", err)
		}
	}
	pressesEnter := func() {
		t.Helper()
		if err := name.Press("Enter"); err != nil {
			t.Fatalf("press Enter in the name: %v", err)
		}
	}

	fills("egg")
	pressesAdd()
	b.IsConfirmed(t, "Added egg.")
	title, err := sheet.Locator(".row-title").BoundingBox()
	if err != nil {
		t.Fatalf("find the row's name: %v", err)
	}
	details, err := sheet.Locator(".row-meta").BoundingBox()
	if err != nil {
		t.Fatalf("find the row's details: %v", err)
	}
	if details.X != title.X || details.Y < title.Y+title.Height {
		t.Errorf("the row's details are at (%v, %v), want under its name at (%v, %v)", details.X, details.Y, title.X, title.Y)
	}

	leavesName("eggs")
	sees("Did you mean egg?")
	leavesName("salmon")
	if err := expect.Locator(sheet).Not().ToContainText("Did you mean"); err != nil {
		t.Fatalf("the suggestion stays for another food: %v", err)
	}
	leavesName("eggs")
	sees("Did you mean egg?")
	search := sheet.GetByRole("searchbox", playwright.LocatorGetByRoleOptions{Name: "Search"})
	if err := search.PressSequentially("zz"); err != nil {
		t.Fatalf("search for zz: %v", err)
	}
	sees("No Ingredient is like zz.")
	if err := expect.Locator(search).ToBeFocused(); err != nil {
		t.Errorf("searching moves the focus: %v", err)
	}
	if err := sheet.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Use egg"}).Click(); err != nil {
		t.Fatalf("pick egg: %v", err)
	}
	b.IsConfirmed(t, "egg is already one of your Ingredients.")
	if got, err := name.InputValue(); err != nil || got != "" {
		t.Errorf("the name after picking egg is %q, %v, want cleared", got, err)
	}
	b.IsAt(t, "/kitchen")

	fills("eggs")
	sees("Did you mean egg?")
	pressesAdd()
	b.IsConfirmed(t, "Added eggs.")

	// Add pressed from the name, held as a finger holds it: the suggestion arrives and
	// moves Add under the press, and one more press adds the name.
	fills("salmon")
	if err := name.Fill("eggys"); err != nil {
		t.Fatalf("change the name: %v", err)
	}
	pressesAdd(playwright.LocatorClickOptions{Delay: playwright.Float(150)})
	sees("Did you mean egg")
	pressesAdd()
	b.IsConfirmed(t, "Added eggys.")

	fills("eggz")
	sees("Did you mean")
	if err := name.Fill("eggy"); err != nil {
		t.Fatalf("change the name: %v", err)
	}
	pressesEnter()
	if err := expect.Locator(name).ToHaveAttribute("aria-invalid", "true"); err != nil {
		t.Fatalf("the changed name isn't matched again: %v", err)
	}
	if err := expect.Locator(name).ToBeFocused(); err != nil {
		t.Errorf("the changed name, matched again, isn't focused: %v", err)
	}
	pressesEnter()
	b.IsConfirmed(t, "Added eggy.")
	b.IsAt(t, "/kitchen")

	fills("petto di pollo")
	pressesAdd()
	b.IsConfirmed(t, "Added petto di pollo.")
	if err := search.Fill(""); err != nil {
		t.Fatalf("clear the search: %v", err)
	}
	if err := search.PressSequentially("polo"); err != nil {
		t.Fatalf("search for polo: %v", err)
	}
	results := sheet.Locator("#ingredient-list")
	if err := expect.Locator(results).Not().ToContainText("egg"); err != nil {
		t.Errorf("searching for polo still lists egg: %v", err)
	}
	if err := expect.Locator(results).ToContainText("petto di pollo"); err != nil {
		t.Errorf("searching for polo doesn't find petto di pollo: %v", err)
	}
	if err := expect.Locator(search).ToBeFocused(); err != nil {
		t.Errorf("the search loses its focus as the User types: %v", err)
	}
}

// The Profile's times of day show the User's Language, not the browser's: an
// English-locale browser (Playwright's default) on an Italian page reads "13:00", where a
// native time input would show "01:00 PM". Saved, the section swaps in place: a time out
// of order gets the focus, and saved ones are confirmed in a floating Notice.
func TestBrowserSavesTheTimesOfDayInPlace(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/profile")
	b.ChooseLanguage(t, "Italiano", "Save Language")
	b.Sees(t, "Profilo")  // the Language saved, before the page is opened again
	b.Open(t, "/profile") // without the Language's Notice, which also says "Profilo salvato."
	b.Sees(t, "Orari")

	section := b.Locator("#profile-times")
	lunch := section.GetByLabel("Pranzo")
	if got, err := lunch.InputValue(); err != nil || got != "13:00" {
		t.Errorf("lunch shows %q (err %v), want 13:00", got, err)
	}
	if got, err := lunch.GetAttribute("type"); err != nil || got != "text" {
		t.Errorf("lunch is a %q input (err %v), want text: a native time input follows the browser's locale", got, err)
	}
	if got, err := lunch.GetAttribute("inputmode"); err != nil || got != "numeric" {
		t.Errorf("inputmode = %q (err %v), want numeric", got, err)
	}

	if err := lunch.Fill("7:00"); err != nil {
		t.Fatalf("fill lunch: %v", err)
	}
	save := section.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Salva gli orari"})
	if err := save.Click(); err != nil {
		t.Fatalf("save the times of day: %v", err)
	}
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(lunch).ToHaveAttribute("aria-invalid", "true"); err != nil {
		t.Fatalf("lunch isn't refused: %v", err)
	}
	if err := expect.Locator(lunch).ToBeFocused(); err != nil {
		t.Errorf("the refused lunch isn't focused: %v", err)
	}
	b.Sees(t, "Deve essere dopo l'ora di colazione.")

	if err := lunch.Fill("12:30"); err != nil {
		t.Fatalf("fill lunch: %v", err)
	}
	if err := save.Click(); err != nil {
		t.Fatalf("save the times of day: %v", err)
	}
	b.IsConfirmed(t, "Profilo salvato.")
	if err := expect.Locator(lunch).ToHaveValue("12:30"); err != nil {
		t.Errorf("the saved lunch isn't shown: %v", err)
	}
	b.Shot(t, "times-of-day")
}

// Ticking an appliance and writing the note on its model, the section saves in place,
// confirmed in a floating Notice, and shows what it saved.
func TestBrowserSavesAnApplianceWithItsNoteInPlace(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/profile")
	section := b.Locator("#profile-appliances")
	airFryer := section.GetByRole("checkbox", playwright.LocatorGetByRoleOptions{Name: "Air fryer"})
	note := section.GetByRole("textbox", playwright.LocatorGetByRoleOptions{Name: "Model of the air fryer"})
	if err := airFryer.Check(); err != nil {
		t.Fatalf("tick the air fryer: %v", err)
	}
	if err := note.Fill("Ninja AF100"); err != nil {
		t.Fatalf("write the air fryer's model: %v", err)
	}
	if _, err := b.Evaluate(`window.notReloaded = true`); err != nil {
		t.Fatalf("mark the page: %v", err)
	}
	save := section.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save appliances"})
	if err := save.Click(); err != nil {
		t.Fatalf("save the appliances: %v", err)
	}
	b.IsConfirmed(t, "Profile saved.")
	if kept, err := b.Evaluate(`window.notReloaded === true`); err != nil || kept != true {
		t.Errorf("saving the appliances reloaded the page (err %v)", err)
	}
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(airFryer).ToBeChecked(); err != nil {
		t.Errorf("the saved air fryer isn't ticked: %v", err)
	}
	if err := expect.Locator(note).ToHaveValue("Ninja AF100"); err != nil {
		t.Errorf("the saved air fryer's model isn't shown: %v", err)
	}
	b.Shot(t, "appliances")
}

// A finger's target is at least 44px tall: a Kitchen row's Count and Move, and a line
// of a Run's Trace that opens.
func TestBrowserTargetsAreAtLeast44pxTall(t *testing.T) {
	s := draftedLasagne(t)
	run := s.Page().Path
	s.Counts("rice", "", "700", "")
	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.IsAt(t, "/")
	tall := func(what string, l playwright.Locator) {
		t.Helper()
		if h := box(t, what, l).Height; h < 44 {
			t.Errorf("%s is %.1fpx tall, want at least 44", what, h)
		}
	}

	b.Open(t, "/kitchen")
	count := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Count rice", Exact: playwright.Bool(true)})
	move := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Move rice in the pantry", Exact: playwright.Bool(true)})
	tall("Count", count)
	tall("Move", move)
	// Side by side, each keeps a target of its own.
	if c, m := box(t, "Count", count), box(t, "Move", move); c.X+c.Width > m.X {
		t.Errorf("Count ends at %.1fpx, over Move, which starts at %.1fpx", c.X+c.Width, m.X)
	}
	b.Open(t, run)
	tall("a Trace line that opens", b.Locator(".fold > summary").First())
}
