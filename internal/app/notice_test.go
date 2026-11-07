package app_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// With no script, a plain post still lands on a page whose floating Notice shows: the
// Profile saved, and a Count from its own page.
func TestBrowserANoticeShowsWithoutJS(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.BrowserWithoutJS(t)
	b.SignUp(t, "ada@example.com")

	b.Open(t, "/profile")
	b.ChooseLanguage(t, "Italiano", "Save")
	b.IsAt(t, "/profile")
	b.IsConfirmed(t, "Profilo salvato.")
	b.Shot(t, "notice-without-js")

	b.Open(t, "/profile")
	if err := playwright.NewPlaywrightAssertions().Locator(b.Notices()).ToHaveCount(0); err != nil {
		t.Errorf("the Profile confirms again when opened again: %v", err)
	}
}

// On a phone with the keyboard up, a Count's floating Notice shows whole at the top of
// the screen, over the Count sheet, its Close a full touch target that, focused, has the
// ring a navy ground needs.
func TestBrowserANoticeOnAPhoneWithTheKeyboardUp(t *testing.T) {
	b := signedUpBrowser(t)
	if err := b.SetViewportSize(390, 844); err != nil {
		t.Fatalf("size a phone: %v", err)
	}
	b.Open(t, "/kitchen")
	addsRiceInTheBrowser(t, b)
	expect := playwright.NewPlaywrightAssertions()
	b.IsConfirmed(t, "Added rice.")
	if _, err := b.Evaluate(`() => document.querySelectorAll("#notices .notice").forEach((n) => n.getAnimations().forEach((a) => a.finish()))`); err != nil {
		t.Fatalf("run adding rice's Notice out: %v", err)
	}
	if err := expect.Locator(b.Notices()).ToHaveCount(0); err != nil {
		t.Fatalf("adding rice's Notice stays once its time has run out: %v", err)
	}

	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	if err := sheet.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "rice"}).Click(); err != nil {
		t.Fatalf("choose rice: %v", err)
	}
	stated := sheet.GetByLabel("How much is there")
	if err := expect.Locator(stated).ToBeFocused(); err != nil {
		t.Fatalf("the quantity isn't focused: %v", err)
	}
	// The keyboard takes the bottom of the screen.
	if err := b.SetViewportSize(390, 480); err != nil {
		t.Fatalf("raise the keyboard: %v", err)
	}
	if err := stated.Fill("700"); err != nil {
		t.Fatalf("state the quantity: %v", err)
	}
	if err := stated.Press("Enter"); err != nil {
		t.Fatalf("save the Count: %v", err)
	}
	b.IsConfirmed(t, "Counted rice: 700 g in the pantry.")
	b.Shot(t, "notice-phone-keyboard")

	notice := b.Notices().First()
	box, err := notice.BoundingBox()
	if err != nil || box == nil {
		t.Fatalf("find the Notice: %v", err)
	}
	if box.Y < 0 || box.X < 0 || box.Y+box.Height > 480 || box.X+box.Width > 390 {
		t.Errorf("the Notice at %+v isn't whole on the screen above the keyboard", *box)
	}
	onTop, err := notice.Evaluate(`(n) => { const r = n.getBoundingClientRect(); return n.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)) }`, nil)
	if err != nil || onTop != true {
		t.Errorf("the Notice isn't over the Count sheet: %v %v", onTop, err)
	}
	closes := notice.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close notice"})
	target, err := closes.BoundingBox()
	if err != nil || target == nil {
		t.Fatalf("find the Notice's Close: %v", err)
	}
	if target.Width < 44 || target.Height < 44 {
		t.Errorf("the Notice's Close is %v×%v, want at least 44×44", target.Width, target.Height)
	}
	if err := closes.Focus(); err != nil {
		t.Fatalf("focus the Notice's Close: %v", err)
	}
	if err := expect.Locator(closes).ToHaveCSS("outline-color", "rgb(235, 160, 131)"); err != nil {
		t.Errorf("the focused Close isn't ringed in salmon: %v", err)
	}
}

// A floating Notice keeps its time as the sheet it shows over closes, from a tap outside
// it: it doesn't start again.
func TestBrowserANoticeKeepsItsTimeAsTheSheetCloses(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/kitchen")
	addsRiceInTheBrowser(t, b)
	expect := playwright.NewPlaywrightAssertions()
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Count"}).First().Click(); err != nil {
		t.Fatalf("open the Count sheet: %v", err)
	}
	if err := sheet.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "rice"}).Click(); err != nil {
		t.Fatalf("choose rice: %v", err)
	}
	if err := sheet.GetByLabel("How much is there").Fill("700"); err != nil {
		t.Fatalf("state the quantity: %v", err)
	}
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("save the Count: %v", err)
	}
	b.IsConfirmed(t, "Counted rice: 700 g in the pantry.")
	notice := b.Notices().Filter(playwright.LocatorFilterOptions{HasText: "Counted rice"})
	// Three seconds of its time have gone.
	if _, err := notice.Evaluate(`(n) => n.getAnimations({ subtree: true }).forEach((a) => { a.currentTime = 3000 })`, nil); err != nil {
		t.Fatalf("move the Notice's time on: %v", err)
	}

	// The screen's left edge, halfway down, is outside the sheet on a desktop.
	if err := b.Mouse().Click(8, 360); err != nil {
		t.Fatalf("tap outside the Count sheet: %v", err)
	}
	if err := expect.Locator(sheet).ToBeHidden(); err != nil {
		t.Fatalf("a tap outside doesn't close the Count sheet: %v", err)
	}
	b.IsConfirmed(t, "Counted rice: 700 g in the pantry.")
	kept, err := notice.Evaluate(`(n) => n.getAnimations().find((a) => a.animationName === "notice-time")?.currentTime >= 3000`, nil)
	if err != nil {
		t.Fatalf("read the Notice's time: %v", err)
	}
	if kept != true {
		t.Errorf("the Notice's time started again as the sheet closed")
	}
}
