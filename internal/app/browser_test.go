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
	b.Sees(t, "Tuesday 3 November")

	b.SetClock(t, tuesday.AddDate(0, 0, 1))
	b.Open(t, "/")
	b.Sees(t, "Wednesday 4 November")
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
	b.ChooseLanguage(t, "English", "Salva")
	b.IsAt(t, "/profile?saved")
	b.Sees(t, "Profile saved.")
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
	b.Sees(t, "Now: lunch at 13:00")
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

// The Ingredients side sheet opens from the Kitchen page, over it, focused on its first
// field, and adds an Ingredient there: the sheet stays open, the Ingredient listed and the form cleared. An add refused
// shows why with focus on the field, the sheet still open and the page under it the same.
func TestBrowserAddsAnIngredientInTheSideSheet(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/")
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Open your kitchen"}).Click(); err != nil {
		t.Fatalf("follow the link to the kitchen: %v", err)
	}
	b.IsAt(t, "/kitchen")
	if err := b.Locator("main").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ingredients"}).Click(); err != nil {
		t.Fatalf("open the Ingredients: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Ingredients"})
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Fatalf("the Ingredients sheet doesn't open: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("Name")).ToBeFocused(); err != nil {
		t.Errorf("opening the sheet doesn't focus its first field: %v", err)
	}
	b.IsAt(t, "/kitchen")

	fill := func(label, value string) {
		t.Helper()
		if err := sheet.GetByLabel(label, playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(true)}).Fill(value); err != nil {
			t.Fatalf("fill %s: %v", label, err)
		}
	}
	choose := func(label, option string) {
		t.Helper()
		if _, err := sheet.GetByLabel(label).SelectOption(playwright.SelectOptionValues{Labels: &[]string{option}}); err != nil {
			t.Fatalf("choose %s: %v", label, err)
		}
	}
	addEgg := func() {
		t.Helper()
		fill("Name", "egg")
		choose("Unit", "piece")
		choose("Usually kept in", "fridge")
		fill("Calories", "70")
		fill("Protein", "6.3")
		fill("Carbs", "0.4")
		fill("Fat", "4.8")
		if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Add"}).Click(); err != nil {
			t.Fatalf("add egg: %v", err)
		}
	}

	addEgg()
	if err := expect.Locator(sheet).ToContainText("Added egg."); err != nil {
		t.Fatalf("the sheet doesn't confirm egg was added: %v", err)
	}
	if err := expect.Locator(sheet).ToContainText("per piece: 70 kcal"); err != nil {
		t.Errorf("the sheet doesn't list egg: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("Name")).ToHaveValue(""); err != nil {
		t.Errorf("the add form isn't cleared after adding: %v", err)
	}
	b.IsAt(t, "/kitchen")

	addEgg()
	if err := expect.Locator(sheet).ToContainText("You already have egg"); err != nil {
		t.Fatalf("the sheet doesn't refuse egg again: %v", err)
	}
	if err := expect.Locator(sheet.GetByLabel("Name")).ToBeFocused(); err != nil {
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
