package app_test

import (
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/apptest"
)

// TestPagesShowTheClocksDay through a real browser: the clock is set
// through the test-only switch.
func TestBrowserPagesShowTheClocksDay(t *testing.T) {
	h := apptest.Start(t)
	h.Invite("ada@example.com")
	b := h.Browser(t)
	b.SignIn(t, "ada@example.com")
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
	b.SignIn(t, "ada@example.com")
	b.Sees(t, "Signed in as ada@example.com")

	b.Click(t, "Sign out")
	b.IsAt(t, "/sign-in")
}
