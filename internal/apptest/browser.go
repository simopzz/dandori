package apptest

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// Browser is a real, headless Chromium on the app, for the few tests of
// each page's main path that need the UI as the User gets it: htmx, CSS,
// scripts. It sets the clock only through POST /_test/clock, as a browser
// test against a separately started server would.
type Browser struct {
	playwright.Page
	url  string
	auth *FakeAuth
}

// Browser opens a browser page on the app. Without Playwright's Chromium
// (`mise run browsers`) the test is skipped, except in CI, where it fails.
func (h *Harness) Browser(t testing.TB) *Browser {
	t.Helper()
	pw, err := playwright.Run(&playwright.RunOptions{Stdout: os.Stderr})
	if err != nil {
		notInstalled(t, "start Playwright", err)
	}
	t.Cleanup(func() {
		if err := pw.Stop(); err != nil {
			t.Logf("stop Playwright: %v", err)
		}
	})
	browser, err := pw.Chromium.Launch()
	if err != nil {
		notInstalled(t, "launch Chromium", err)
	}
	t.Cleanup(func() { browser.Close() })
	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("open a page: %v", err)
	}
	return &Browser{Page: page, url: h.URL, auth: h.Auth}
}

// notInstalled skips the test when what failed is likely a browser not
// installed here, and fails it in CI, which installs one.
func notInstalled(t testing.TB, what string, err error) {
	t.Helper()
	if os.Getenv("CI") == "" {
		t.Skipf("%s (run `mise run browsers`): %v", what, err)
	}
	t.Fatalf("%s: %v", what, err)
}

// SetClock sets the app's clock to now through the test-only switch.
func (b *Browser) SetClock(t testing.TB, now time.Time) {
	t.Helper()
	res, err := b.Request().Post(b.url+"/_test/clock", playwright.APIRequestContextPostOptions{
		Form: map[string]any{"now": now.Format(time.RFC3339)},
	})
	if err != nil {
		t.Fatalf("set the clock: %v", err)
	}
	if res.Status() != http.StatusNoContent {
		body, _ := res.Text()
		t.Fatalf("set the clock: status %d: %s", res.Status(), body)
	}
}

// Open goes to path, as typed into the address bar.
func (b *Browser) Open(t testing.TB, path string) {
	t.Helper()
	res, err := b.Goto(b.url + path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if res.Status() != http.StatusOK {
		t.Fatalf("open %s: status %d", path, res.Status())
	}
}

// SignIn signs the invited email in as a User does: asks for a magic link, opens it
// from their mailbox and confirms, landing on Today, or at a first sign-in on the
// Language choice.
func (b *Browser) SignIn(t testing.TB, email string) {
	t.Helper()
	b.Open(t, "/sign-in")
	if err := b.GetByLabel("Email").Fill(email); err != nil {
		t.Fatalf("type the email: %v", err)
	}
	b.Click(t, "Email me a sign-in link")
	b.IsAt(t, "/sign-in/sent")
	if _, err := b.Goto(b.auth.LastLink(t, email)); err != nil {
		t.Fatalf("open the magic link: %v", err)
	}
	b.Click(t, "Sign in to Dandori")
}

// SignUp signs the invited email in for the first time and chooses English, landing
// on Today.
func (b *Browser) SignUp(t testing.TB, email string) {
	t.Helper()
	b.SignIn(t, email)
	b.IsAt(t, "/language")
	b.ChooseLanguage(t, "English", "Continue")
	b.IsAt(t, "/")
}

// ChooseLanguage picks the Language named name on the page and submits it with the button
// named submit.
func (b *Browser) ChooseLanguage(t testing.TB, name, submit string) {
	t.Helper()
	if err := b.GetByRole("radio", playwright.PageGetByRoleOptions{Name: name}).Check(); err != nil {
		t.Fatalf("choose %s: %v", name, err)
	}
	b.Click(t, submit)
}

// Click clicks the button named name.
func (b *Browser) Click(t testing.TB, name string) {
	t.Helper()
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: name}).Click(); err != nil {
		t.Fatalf("click %q: %v", name, err)
	}
}

// IsAt waits for the page to be at path.
func (b *Browser) IsAt(t testing.TB, path string) {
	t.Helper()
	if err := playwright.NewPlaywrightAssertions().Page(b.Page).ToHaveURL(b.url + path); err != nil {
		t.Fatalf("the page is not at %s: %v", path, err)
	}
}

// Sees checks the page's main content shows want.
func (b *Browser) Sees(t testing.TB, want string) {
	t.Helper()
	if err := playwright.NewPlaywrightAssertions().Locator(b.Locator("main")).ToContainText(want); err != nil {
		t.Errorf("%s does not show %q: %v", b.URL(), want, err)
	}
}
