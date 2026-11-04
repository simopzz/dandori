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
