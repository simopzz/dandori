package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// From the start of the Delivery's window Today lists where each line of its Grocery
// List goes, a freezer split as two lines, each with its Count and move shown but
// unavailable until the Delivery arrives.
func TestTodayListsThePutAwayFromTheWindowsStart(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.At("Thu 17:59")
	s.Open("/")
	s.DoesNotSee("Put the Delivery away")

	s.At("Thu 18:00")
	s.Open("/")
	s.Sees("Put the Delivery away")
	s.Sees("yogurt 50 g in the fridge Count yogurt Move yogurt in the fridge")
	s.Sees("yogurt 100 g in the freezer Count yogurt Move yogurt in the freezer")
	s.DoesNotSee("chicken breast")
	for _, path := range []string{"/kitchen/count", "/kitchen/move"} {
		if strings.Contains(s.Page().Body, `href="`+path+"?") {
			t.Errorf("Today links to %s before the Delivery arrives", path)
		}
	}
}

// From arrival a put-away line's Count and move fix what came otherwise, reaching the
// Kitchen page, and land back on Today without htmx. A Stock Item holding nothing any
// more can't be moved.
func TestAPutAwayLineIsCountedAndMovedFromToday(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.At("Thu 20:00")
	s.Open("/")
	s.Follows("Count yogurt") // the fridge's line, the first
	s.SubmitsWith("count-form", map[string]string{"stated": "200"})
	s.IsAt("/")
	s.IsConfirmed("Counted yogurt: 200 g in the fridge.")

	s.Follows("Move yogurt in the freezer")
	s.SubmitsWith("move-form", map[string]string{"to": "fridge"})
	s.IsAt("/")
	s.IsConfirmed("Moved 100 g of yogurt to the fridge.")
	s.Open("/kitchen")
	s.Sees("Fridge yogurt 300 g")
	s.DoesNotSee("Move yogurt in the freezer")
	s.Open("/")
	s.Sees("yogurt 100 g in the freezer Count yogurt Move yogurt in the freezer")
	if regexp.MustCompile(`href="/kitchen/move\?[^"]*location=freezer`).MatchString(s.Page().Body) {
		t.Error("Today moves a Stock Item that holds nothing")
	}

	// With htmx, the Count sheet answers in place, on Today.
	sheet := s.HTMXGet(t, apptest.SheetSource(t, s.Page().Body, "Count yogurt"))
	count := apptest.Form(t, sheet.Body, "count-form")
	count.Values.Set("stated", "250")
	if got := s.HTMXPostForm(t, count.Action, count.Values); got.StatusCode != http.StatusOK || !strings.Contains(apptest.Text(got.Body), "Counted yogurt: 250 g in the fridge.") {
		t.Fatalf("Count from Today's sheet: %d %s", got.StatusCode, apptest.Text(got.Body))
	}
	s.Open("/kitchen")
	s.Sees("Fridge yogurt 250 g")
}

// The put-away list stays on Today until the end of the day after the Delivery's day,
// for a Delivery unpacked the next morning; after that, the Kitchen page corrects it.
func TestThePutAwayListGoesAfterTheDayAfterTheDelivery(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.At("Fri 23:59")
	s.Open("/")
	s.Sees("Put the Delivery away")
	s.At("Sat 00:00")
	s.Open("/")
	s.DoesNotSee("Put the Delivery away")
}

// Today's put-away list shows its Counts unavailable until the Delivery arrives; then it
// opens a line's Count in its sheet, over Today, and keeps the
// User there once it is saved.
func TestBrowserCountsAPutAwayLineOnToday(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.At("Thu 18:30")
	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/")
	b.Sees(t, "Count and Move open once the Delivery arrives")
	b.Shot(t, "today-put-away-before")

	s.At("Thu 20:00")
	b.Open(t, "/")
	b.SeesInTurn(t, "Put the Delivery away", "yogurt", "50 g", "in the fridge", "yogurt", "100 g", "in the freezer")
	b.Shot(t, "today-put-away")

	expect := playwright.NewPlaywrightAssertions()
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Count yogurt"}).First().Click(); err != nil {
		t.Fatalf("open the fridge line's Count: %v", err)
	}
	sheet := b.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Count"})
	if err := sheet.GetByLabel("How much is there").Fill("200"); err != nil {
		t.Fatalf("state the quantity: %v", err)
	}
	b.Shot(t, "today-put-away-count")
	if err := sheet.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Count"}).Click(); err != nil {
		t.Fatalf("save the Count: %v", err)
	}
	b.IsConfirmed(t, "Counted yogurt: 200 g in the fridge.")
	if err := expect.Locator(sheet).ToBeVisible(); err != nil {
		t.Errorf("the Count sheet closed: %v", err)
	}
	b.IsAt(t, "/")
}
