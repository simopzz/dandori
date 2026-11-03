package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// What a page shows follows the app's clock, not the machine's: a page
// opened on Tuesday shows Tuesday, and the same page a day later shows
// Wednesday.
func TestPagesShowTheClocksDay(t *testing.T) {
	s := apptest.NewScenario(t)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")

	s.At("Tue 10:00")
	s.Open("/")
	s.IsAt("/")
	s.Sees("Tuesday 3 November")

	s.NextDay()
	s.Open("/")
	s.Sees("Wednesday 4 November")
}
