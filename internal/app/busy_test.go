package app_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// A write slow to answer shows the busy bar and the pressed button busy, both gone
// once it lands.
func TestBrowserShowsASlowWriteIsBusy(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/profile")
	held := b.Hold(t, "/profile/language")

	b.ChooseLanguage(t, "English", "Save")
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(busyBar(b)).ToBeVisible(); err != nil {
		t.Errorf("the busy bar doesn't show while the write is slow: %v", err)
	}
	if err := expect.Locator(save(b)).ToHaveAttribute("aria-busy", "true"); err != nil {
		t.Errorf("Save isn't busy while the write is slow: %v", err)
	}

	held.Release()
	b.IsAt(t, "/profile?saved")
	if err := expect.Locator(busyBar(b)).ToBeHidden(); err != nil {
		t.Errorf("the busy bar stays after the write: %v", err)
	}
	if err := expect.Locator(save(b)).Not().ToHaveAttribute("aria-busy", "true"); err != nil {
		t.Errorf("Save stays busy after the write: %v", err)
	}
}

// A slow boosted navigation shows the busy bar too, not only a form's post.
func TestBrowserShowsASlowNavigationIsBusy(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/")
	held := b.Hold(t, "/profile")

	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Profile"}).Click(); err != nil {
		t.Fatalf("follow the Profile tab: %v", err)
	}
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(busyBar(b)).ToBeVisible(); err != nil {
		t.Errorf("the busy bar doesn't show while the navigation is slow: %v", err)
	}

	held.Release()
	b.IsAt(t, "/profile")
	if err := expect.Locator(busyBar(b)).ToBeHidden(); err != nil {
		t.Errorf("the busy bar stays after the navigation: %v", err)
	}
}

// A write answered at once shows neither the bar nor a busy button, so a quick click
// doesn't flicker.
func TestBrowserShowsNothingForAQuickWrite(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/profile")
	if _, err := b.Evaluate(`([bar, busy]) => {
		window.sawBusy = false;
		const watch = () => {
			const shown = document.querySelector(bar);
			if ((shown && getComputedStyle(shown).visibility === "visible") || document.querySelector(busy)) {
				window.sawBusy = true;
			}
			requestAnimationFrame(watch);
		};
		watch();
	}`, []string{busyBarSelector, busyButtonSelector}); err != nil {
		t.Fatalf("watch for the busy state: %v", err)
	}

	b.ChooseLanguage(t, "English", "Save")
	b.IsAt(t, "/profile?saved")
	b.Sees(t, "Profile saved.")
	if saw, err := b.Evaluate(`window.sawBusy`); err != nil || saw != false {
		t.Errorf("a quick write showed the busy state (saw %v, err %v)", saw, err)
	}
}

// A slow write that fails takes the busy state down with it, and frees Save to try again.
func TestBrowserShowsAFailedWriteIsNoLongerBusy(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/profile")
	held := b.Hold(t, "/profile/language")

	b.ChooseLanguage(t, "English", "Save")
	expect := playwright.NewPlaywrightAssertions()
	if err := expect.Locator(busyBar(b)).ToBeVisible(); err != nil {
		t.Fatalf("the busy bar doesn't show while the write is slow: %v", err)
	}

	held.Fail()
	if err := expect.Locator(busyBar(b)).ToBeHidden(); err != nil {
		t.Errorf("the busy bar stays after the write failed: %v", err)
	}
	if err := expect.Locator(save(b)).Not().ToHaveAttribute("aria-busy", "true"); err != nil {
		t.Errorf("Save stays busy after the write failed: %v", err)
	}
	if err := expect.Locator(save(b)).ToBeEnabled(); err != nil {
		t.Errorf("Save stays disabled after the write failed: %v", err)
	}
}

// A double click on a write sends it once.
func TestBrowserSendsADoubleClickedWriteOnce(t *testing.T) {
	b := signedUpBrowser(t)
	b.Open(t, "/profile")
	held := b.Hold(t, "/profile/language")

	if err := b.GetByRole("radio", playwright.PageGetByRoleOptions{Name: "English"}).Check(); err != nil {
		t.Fatalf("choose English: %v", err)
	}
	if err := save(b).Dblclick(); err != nil {
		t.Fatalf("double-click Save: %v", err)
	}
	held.Release()
	b.IsAt(t, "/profile?saved")
	if n := held.Count(); n != 1 {
		t.Errorf("a double-clicked Save sent %d requests, want 1", n)
	}
}

func signedUpBrowser(t *testing.T) *apptest.Browser {
	t.Helper()
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignUp(t, "ada@example.com")
	return b
}

// The busy bar, and a button marked busy.
const (
	busyBarSelector    = ".busy-bar"
	busyButtonSelector = "[aria-busy=true]"
)

func busyBar(b *apptest.Browser) playwright.Locator { return b.Locator(busyBarSelector) }

func save(b *apptest.Browser) playwright.Locator {
	return b.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Save"})
}
