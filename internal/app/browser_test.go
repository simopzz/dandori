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
	b := h.Browser(t)
	tuesday := time.Date(2026, time.November, 3, 10, 0, 0, 0, h.Clock.Now().Location())

	b.SetClock(t, tuesday)
	b.Open(t, "/")
	b.Sees(t, "Tuesday 3 November")

	b.SetClock(t, tuesday.AddDate(0, 0, 1))
	b.Open(t, "/")
	b.Sees(t, "Wednesday 4 November")
}
