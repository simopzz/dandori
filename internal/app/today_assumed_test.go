package app_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/apptest"
)

// Once a day's Slots pass, Today lists what Dandori assumed since the User last looked,
// by day, oldest first: Thursday evening's thaw of the lasagne in a row of its own, then
// Friday's Slots, each summed up and opening to its Movements. Thursday's Delivery isn't
// among them: the put-away list shows it.
func TestTodayListsWhatWasAssumedByDay(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.At("Fri 21:00")
	s.Open("/")
	s.Sees("Since you last looked Thursday, November 5 Out of the freezer: lasagne Dinner 8:00 pm for tomorrow lasagne × 1 Thawed in the fridge" +
		" Friday, November 6 yogurt prepared, 1 eaten Breakfast 8:00 am yogurt 150 g Used up in the fridge")
	s.Sees("Rice bowl prepared, 1 eaten, 2 kept Lunch 1:00 pm rice 375 g Used up in the pantry Rice bowl × 2 Prepared in the fridge" +
		" Rice bowl × 1 Prepared in the freezer Rice bowl × 1 Eaten in the fridge")
	s.Sees("lasagne eaten Snack 5:00 pm")
	s.Sees("Rice bowl eaten Dinner 8:00 pm")
	s.DoesNotSee("Delivered")
}

// Visiting again lists only what was assumed since, but never less than all of the day's:
// on Friday afternoon Thursday's thaw is gone and the breakfast stays, also on a reload.
func TestTodayListsWhatWasAssumedSinceTheLastVisitAndAllOfToday(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.At("Fri 09:00")
	s.Open("/")
	s.Sees("Out of the freezer: lasagne")
	s.Sees("yogurt prepared, 1 eaten")
	s.At("Fri 14:00")
	for range 2 { // the second a reload
		s.Open("/")
		s.DoesNotSee("Thursday, November 5")
		s.Sees("Since you last looked Friday, November 6 yogurt prepared, 1 eaten Breakfast 8:00 am")
		s.Sees("Rice bowl prepared, 1 eaten, 2 kept Lunch 1:00 pm")
	}
}

// After three days without a visit, all three days are listed: the Cycle from Thursday's
// Delivery to Sunday sets out yogurt at every Slot.
func TestTodayListsEveryDayAssumedWhileAway(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approves(t, s, func(req agent.Request) map[string]any {
		return map[string]any{"prep_sessions": yogurtAt(idIn(t, req, `Ingredient (\d+): yogurt`), []string{"2026-11-06", "2026-11-07", "2026-11-08"})}
	}, func() {
		s.OpensCycle(apptest.Cycle{Delivery: "11/05/2026", From: "6pm", To: "8pm", Ends: "11/08/2026"})
		s.PlansCycle()
	})

	s.At("Thu 21:00")
	s.Open("/")
	s.DoesNotSee("Since you last looked")
	s.At("Sun 21:00")
	s.Open("/")
	s.Sees("Since you last looked Friday, November 6 yogurt prepared, 1 eaten Breakfast 8:00 am")
	s.Sees("Saturday, November 7 yogurt prepared, 1 eaten Breakfast")
	s.Sees("Sunday, November 8 yogurt prepared, 1 eaten Breakfast")
}

// Each Movement listed links to its Stock Item or Portion Lot on the Kitchen page.
func TestWhatTodayListsAsAssumedLinksToTheKitchen(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.At("Fri 09:00")
	s.Open("/")
	s.Follows("lasagne")
	s.Sees("lasagne · Fridge Thawed 0 → 1 Thu, Nov 5 · 8:00 pm")
	s.At("Fri 14:00")
	s.Open("/")
	s.Follows("rice")
	s.Sees("rice · Pantry Used up 800 g → 425 g Fri, Nov 6 · 1:00 pm")
	s.Open("/")
	s.Follows("Rice bowl") // the two prepared in the fridge, one of them eaten at lunch
	s.Sees("Rice bowl · Fridge Eaten 2 → 1 Fri, Nov 6 · 1:00 pm Prepared 0 → 2 Fri, Nov 6 · 1:00 pm")
}

// In a browser, an assumed Slot's row opens to its Movements.
func TestBrowserOpensAnAssumedSlot(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Fri 14:00")
	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/")
	b.SeesInTurn(t, "Since you last looked", "Friday, November 6", "Rice bowl prepared, 1 eaten, 2 kept")

	expect := playwright.NewPlaywrightAssertions()
	usedUp := b.Locator("details", playwright.PageLocatorOptions{HasText: "Rice bowl prepared"}).GetByText("Used up")
	if err := expect.Locator(usedUp).Not().ToBeVisible(); err != nil {
		t.Errorf("the lunch's Movements show before it is opened: %v", err)
	}
	if err := b.Locator("summary", playwright.PageLocatorOptions{HasText: "Rice bowl prepared"}).Click(); err != nil {
		t.Fatalf("open the lunch's row: %v", err)
	}
	if err := expect.Locator(usedUp).ToBeVisible(); err != nil {
		t.Errorf("opened, the lunch's row doesn't show the rice used up: %v", err)
	}
	b.Shot(t, "today-assumed")
}
