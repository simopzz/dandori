package apptest

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	url    string
	auth   *FakeAuth
	settle func()
}

// Browser opens a browser page on the app. Without Playwright's Chromium
// (`mise run browsers`) the test is skipped, except in CI, where it fails.
func (h *Harness) Browser(t testing.TB) *Browser {
	t.Helper()
	return h.browser(t, playwright.BrowserNewPageOptions{})
}

// BrowserWithoutJS is a Browser with JavaScript turned off, as the pages are before htmx
// loads or when it fails: what a page shows with no script.
func (h *Harness) BrowserWithoutJS(t testing.TB) *Browser {
	t.Helper()
	return h.browser(t, playwright.BrowserNewPageOptions{JavaScriptEnabled: playwright.Bool(false)})
}

func (h *Harness) browser(t testing.TB, options playwright.BrowserNewPageOptions) *Browser {
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
	page, err := browser.NewPage(options)
	if err != nil {
		t.Fatalf("open a page: %v", err)
	}
	failOnPageErrors(t, page)
	return &Browser{Page: page, url: h.URL, auth: h.Auth, settle: h.Settle}
}

// failOnPageErrors fails the test, as it ends, for each error page's scripts threw and
// nothing caught, which the User would only see in the console.
func failOnPageErrors(t testing.TB, page playwright.Page) {
	var (
		mu     sync.Mutex
		thrown []error
	)
	page.OnPageError(func(err error) {
		mu.Lock()
		defer mu.Unlock()
		thrown = append(thrown, err)
	})
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, err := range thrown {
			t.Errorf("the page threw: %v", err)
		}
	})
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

// Held is the browser's requests to one path, held back as a server slow to answer would,
// until Release lets them through or Fail drops them.
type Held struct {
	release chan struct{}
	once    sync.Once
	fail    atomic.Bool
	count   atomic.Int32
}

// Hold holds every request the browser makes to path, from now until the returned Held
// is released, and releases them when the test ends.
func (b *Browser) Hold(t testing.TB, path string) *Held {
	t.Helper()
	h := &Held{release: make(chan struct{})}
	err := b.Route(b.url+path, func(r playwright.Route) {
		h.count.Add(1)
		<-h.release
		if h.fail.Load() {
			if err := r.Abort(); err != nil {
				t.Logf("drop %s: %v", path, err)
			}
			return
		}
		if err := r.Continue(); err != nil {
			t.Logf("let %s through: %v", path, err)
		}
	})
	if err != nil {
		t.Fatalf("hold %s: %v", path, err)
	}
	t.Cleanup(h.Release)
	return h
}

// Release lets the held requests through, and any made after.
func (h *Held) Release() { h.once.Do(func() { close(h.release) }) }

// Fail drops the held requests, and any made after, as a lost connection would.
func (h *Held) Fail() {
	h.fail.Store(true)
	h.Release()
}

// Count is how many requests were held.
func (h *Held) Count() int { return int(h.count.Load()) }

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

// ChooseLanguage picks the Language named name on the page and submits it with its form's
// button named submit.
func (b *Browser) ChooseLanguage(t testing.TB, name, submit string) {
	t.Helper()
	radio := b.GetByRole("radio", playwright.PageGetByRoleOptions{Name: name})
	if err := radio.Check(); err != nil {
		t.Fatalf("choose %s: %v", name, err)
	}
	form := b.Locator("form").Filter(playwright.LocatorFilterOptions{Has: radio})
	if err := form.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: submit, Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatalf("click %q: %v", submit, err)
	}
}

// Click clicks the button named name, exactly.
func (b *Browser) Click(t testing.TB, name string) {
	t.Helper()
	if err := b.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: playwright.Bool(true)}).Click(); err != nil {
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

// Shot saves what the window shows as name.png in the folder DANDORI_SHOTS names, doing
// nothing when it names none. Take it after an assertion, which waits for the swap.
func (b *Browser) Shot(t testing.TB, name string) {
	t.Helper()
	b.shot(t, name, false)
}

// ShotPage is Shot of the whole page, below the window too.
func (b *Browser) ShotPage(t testing.TB, name string) {
	t.Helper()
	b.shot(t, name, true)
}

func (b *Browser) shot(t testing.TB, name string, whole bool) {
	t.Helper()
	dir := os.Getenv("DANDORI_SHOTS")
	if dir == "" {
		return
	}
	path := filepath.Join(dir, name+".png")
	if _, err := b.Screenshot(playwright.PageScreenshotOptions{Path: &path, FullPage: &whole}); err != nil {
		t.Fatalf("save %s: %v", path, err)
	}
}

// FollowsRun waits for the page to be a Run's, as starting one lands on, then for every
// Run going to end, and opens the Run's page again to show how it ended: the live page
// may have gone on to what the Run left.
func (b *Browser) FollowsRun(t testing.TB) {
	t.Helper()
	if err := b.WaitForURL(b.url + "/runs/*"); err != nil {
		t.Fatalf("the page is not a Run's: %v", err)
	}
	run := b.URL()
	b.settle()
	if _, err := b.Goto(run); err != nil {
		t.Fatalf("open the Run's page again: %v", err)
	}
}

// Sees checks the page's main content shows want.
func (b *Browser) Sees(t testing.TB, want string) {
	t.Helper()
	if err := playwright.NewPlaywrightAssertions().Locator(b.Locator("main")).ToContainText(want); err != nil {
		t.Errorf("%s does not show %q: %v", b.URL(), want, err)
	}
}

// Notices are the floating Notices, newest first, wherever on the page they are.
func (b *Browser) Notices() playwright.Locator { return b.Locator("#notices .notice") }

// IsConfirmed checks a floating Notice says want, once it has dropped in.
func (b *Browser) IsConfirmed(t testing.TB, want string) {
	t.Helper()
	notice := playwright.NewPlaywrightAssertions().Locator(b.Notices().Filter(playwright.LocatorFilterOptions{HasText: want}))
	if err := notice.ToBeVisible(); err != nil {
		t.Errorf("%s does not confirm %q: %v", b.URL(), want, err)
		return
	}
	if err := notice.ToHaveCSS("opacity", "1"); err != nil {
		t.Errorf("%s does not show its Notice %q: %v", b.URL(), want, err)
	}
}
